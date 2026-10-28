package memory

import (
	"context"
	"math"
	"time"

	"tccelta/backend/internal/domain/dongle"
	"tccelta/backend/internal/domain/ingestion"
	"tccelta/backend/internal/domain/occurrence"
	"tccelta/backend/internal/domain/pid"
	"tccelta/backend/internal/domain/protocol"
	"tccelta/backend/internal/domain/timeline"
	"tccelta/backend/internal/port"
)

// Boot é uma linha da tabela boot, só com o que o worker e o reprocessamento usam. Os testes a montam e conferem.
type Boot struct {
	timeline.PendingBoot
	LastSeq           *int64
	BaseUncertaintyMs *int32
	Pending           bool
	PendingSince      time.Time
	TimeUpdatedAt     time.Time
	GoldPending       bool
	Anchors           []timeline.Anchor
}

func (b *Database) bootRow(k timeline.BootKey) *Boot {
	for _, x := range b.BootRows {
		if x.BootKey == k {
			return x
		}
	}
	return nil
}

func (b *Database) cloneBootRows() []*Boot {
	out := make([]*Boot, len(b.BootRows))
	for i, x := range b.BootRows {
		c := *x
		out[i] = &c
	}
	return out
}

func (r *boots) TakePending(context.Context) (timeline.PendingBoot, bool, error) {
	var best *Boot
	for _, x := range r.BootRows {
		if x.Pending && (best == nil || x.PendingSince.Before(best.PendingSince)) {
			best = x
		}
	}
	if best == nil {
		return timeline.PendingBoot{}, false, nil
	}
	return best.PendingBoot, true, nil
}

func (r *boots) ListAnchors(_ context.Context, k timeline.BootKey) ([]timeline.Anchor, error) {
	if x := (*Database)(r).bootRow(k); x != nil {
		return x.Anchors, nil
	}
	return nil, nil
}

func (r *boots) neighbor(dongleID int32, firstSeq int64, previous bool) *timeline.NeighborReference {
	var best *Boot
	for _, x := range r.BootRows {
		if x.DongleID != dongleID || x.TimeSource == nil || *x.TimeSource != "anchors" || x.FirstSeq == nil {
			continue
		}
		fs := *x.FirstSeq
		switch {
		case previous && fs < firstSeq && (best == nil || fs > *best.FirstSeq):
			best = x
		case !previous && fs > firstSeq && (best == nil || fs < *best.FirstSeq):
			best = x
		}
	}
	if best == nil || best.AMs == nil || best.B == nil || best.BaseUncertaintyMs == nil || best.FirstTMs == nil || best.LastTMs == nil {
		return nil
	}
	return &timeline.NeighborReference{AMs: *best.AMs, B: *best.B, BaseUncertaintyMs: int(*best.BaseUncertaintyMs),
		FirstTMs: *best.FirstTMs, LastTMs: *best.LastTMs}
}

func (r *boots) PreviousAnchored(_ context.Context, dongleID int32, firstSeq int64) (*timeline.NeighborReference, error) {
	return r.neighbor(dongleID, firstSeq, true), nil
}

func (r *boots) NextAnchored(_ context.Context, dongleID int32, firstSeq int64) (*timeline.NeighborReference, error) {
	return r.neighbor(dongleID, firstSeq, false), nil
}

func (r *boots) UpdateTime(_ context.Context, k timeline.BootKey, est *timeline.Estimate, now time.Time) error {
	x := (*Database)(r).bootRow(k)
	if x == nil {
		return nil
	}
	x.AMs, x.B, x.BaseUncertaintyMs, x.TimeSource = nil, nil, nil, nil
	if est != nil {
		a, b, u, s := est.AMs, est.B, int32(est.BaseUncertaintyMs), est.Source
		x.AMs, x.B, x.BaseUncertaintyMs, x.TimeSource = &a, &b, &u, &s
	}
	x.TimeUpdatedAt, x.Pending = now, false
	return nil
}

func (r *boots) MarkAnchorlessNeighbors(_ context.Context, dongleID int32, firstSeq int64, now time.Time) (int64, error) {
	lo, hi := int64(0), int64(math.MaxInt64)
	for _, x := range r.BootRows {
		if x.DongleID != dongleID || x.TimeSource == nil || *x.TimeSource != "anchors" || x.FirstSeq == nil {
			continue
		}
		if fs := *x.FirstSeq; fs < firstSeq {
			lo = max(lo, fs)
		} else if fs > firstSeq {
			hi = min(hi, fs)
		}
	}
	var n int64
	for _, x := range r.BootRows {
		if x.DongleID != dongleID || x.Pending || x.FirstSeq == nil || (x.TimeSource != nil && *x.TimeSource == "anchors") {
			continue
		}
		if fs := *x.FirstSeq; fs > lo && fs < hi {
			x.Pending, x.PendingSince = true, now
			n++
		}
	}
	return n, nil
}

func (r *boots) MarkGold(_ context.Context, dongleID int32, fromSeq, toSeq int64) error {
	for _, x := range r.BootRows {
		if x.DongleID == dongleID && x.FirstSeq != nil && x.LastSeq != nil && *x.FirstSeq <= toSeq && *x.LastSeq >= fromSeq {
			x.GoldPending = true
		}
	}
	return nil
}

func (r *boots) CountPending(context.Context) (int64, error) {
	var n int64
	for _, x := range r.BootRows {
		if x.Pending {
			n++
		}
	}
	return n, nil
}

type anchorGroups Database

func (r *anchorGroups) Delete(_ context.Context, k timeline.BootKey) error {
	delete(r.Groups, k)
	return nil
}

func (r *anchorGroups) Copy(_ context.Context, k timeline.BootKey, _ time.Time, groups []timeline.Group) error {
	r.Groups[k] = append(r.Groups[k], groups...)
	return nil
}

func (r *challenges) Expire(_ context.Context, now time.Time) (int64, error) {
	var n int64
	for k, d := range r.challenges {
		if d.AnsweredAt == nil && d.Result == "" && d.ExpiresAt.Before(now) {
			d.Result = "expired"
			r.challenges[k] = d
			n++
		}
	}
	return n, nil
}

func (r *dongles) Register(_ context.Context, g dongle.Registration) (int32, error) {
	p, err := protocol.ProfileByName(g.Profile)
	if err != nil {
		return 0, err
	}
	var id int32
	for _, d := range r.dongles {
		id = max(id, d.ID)
	}
	id++
	(*Database)(r).AddDongle(dongle.Dongle{ID: id, DevID: g.DevID, Profile: p, AuthKey: g.AuthKey, RecKey: g.RecKey,
		PublicKey: g.PublicKey, Active: true})
	return id, nil
}

// InsertAlertIfNew do banco ignora alertas do mesmo boot e tipo nas últimas 24 h; aqui, qualquer um já registrado conta.
func (r *occurrences) InsertAlertIfNew(_ context.Context, a occurrence.Alert, _ time.Time) error {
	for _, x := range r.Alerts {
		if x.DongleID == a.DongleID && x.Type == a.Type && x.BootGeneration != nil && x.BootID != nil &&
			*x.BootGeneration == *a.BootGeneration && *x.BootID == *a.BootID {
			return nil
		}
	}
	r.Alerts = append(r.Alerts, a)
	return nil
}

func (r *blocks) CopyReadingsAt(_ context.Context, _ int32, ls []port.ReadingAt) error {
	for _, l := range ls {
		r.Readings = append(r.Readings, l.Reading)
	}
	return nil
}

func (r *blocks) CopyPositionsAt(_ context.Context, _ int32, ps []port.PositionAt) error {
	for _, p := range ps {
		r.Positions = append(r.Positions, p.Position)
	}
	return nil
}

func (r *blocks) ListData(_ context.Context, id int32, fromSeq, toSeq int64, limit int32) ([]ingestion.StoredBlock, error) {
	var out []ingestion.StoredBlock
	for _, x := range r.DataBlocks[id] {
		if x.Seq >= fromSeq && x.Seq <= toSeq && int32(len(out)) < limit {
			out = append(out, x)
		}
	}
	return out, nil
}

// CountReadings, DeleteReadings e DeletePositions ignoram dongle e recebimento: as linhas em memória guardam só o bloco de origem.
func (r *blocks) CountReadings(_ context.Context, _ int32, fromSeq, toSeq int64) (int64, error) {
	var n int64
	for _, l := range r.Readings {
		if l.BlockSeq >= fromSeq && l.BlockSeq <= toSeq {
			n++
		}
	}
	return n, nil
}

func (r *blocks) DeleteReadings(_ context.Context, _ int32, fromSeq, toSeq int64, _, _ time.Time) (int64, error) {
	var keep []pid.Reading
	for _, l := range r.Readings {
		if l.BlockSeq < fromSeq || l.BlockSeq > toSeq {
			keep = append(keep, l)
		}
	}
	n := int64(len(r.Readings) - len(keep))
	r.Readings = keep
	return n, nil
}

func (r *blocks) DeletePositions(_ context.Context, _ int32, fromSeq, toSeq int64, _, _ time.Time) (int64, error) {
	var keep []pid.Position
	for _, p := range r.Positions {
		if p.BlockSeq < fromSeq || p.BlockSeq > toSeq {
			keep = append(keep, p)
		}
	}
	n := int64(len(r.Positions) - len(keep))
	r.Positions = keep
	return n, nil
}
