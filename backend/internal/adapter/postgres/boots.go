package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"tccelta/backend/internal/database/dbgen"
	"tccelta/backend/internal/domain/handshake"
	"tccelta/backend/internal/domain/ingestion"
	"tccelta/backend/internal/domain/timeline"
)

type boots struct{ q dbgen.Querier }

func (r *boots) LastTMs(ctx context.Context, dongleID int32, bootIDs []int32, gMin, gMax int32) ([]timeline.RefBoot, error) {
	rows, err := r.q.ReadLastBootTMs(ctx, dongleID, bootIDs, gMin, gMax)
	if err != nil {
		return nil, err
	}
	out := make([]timeline.RefBoot, 0, len(rows))
	for _, x := range rows {
		if x.LastTMs != nil {
			out = append(out, timeline.RefBoot{Generation: x.BootGeneration, BootID: x.BootID, LastTMs: *x.LastTMs})
		}
	}
	return out, nil
}

func (r *boots) EnsurePending(ctx context.Context, dongleID, generation, bootID int32, now time.Time) error {
	return r.q.EnsurePendingBoot(ctx, dongleID, generation, bootID, now)
}

func (r *boots) UpdateIngestion(ctx context.Context, dongleID int32, now time.Time, aggs []ingestion.BootAggregate) error {
	p := dbgen.UpdateIngestionBootsParams{DongleID: dongleID, Now: now}
	for _, x := range aggs {
		p.Generations = append(p.Generations, x.Generation)
		p.BootIds = append(p.BootIds, x.BootID)
		p.FromSeqs = append(p.FromSeqs, x.FromSeq)
		p.ToSeqs = append(p.ToSeqs, x.ToSeq)
		p.TStarts = append(p.TStarts, x.TStart)
		p.TEnds = append(p.TEnds, x.TEnd)
		p.MaxOffsets = append(p.MaxOffsets, x.MaxOffset)
	}
	return r.q.UpdateIngestionBoots(ctx, p)
}

func (r *boots) InsertAnchor(ctx context.Context, a handshake.Anchor) error {
	return r.q.InsertAnchor(ctx, dbgen.InsertAnchorParams{DongleID: a.DongleID, BootGeneration: a.BootGeneration, BootID: a.BootID,
		TMs: a.TMs, Nonce: a.Nonce, S1: a.S1, S2: a.S2})
}

func (r *boots) NotifyPending(ctx context.Context, key string) error {
	return r.q.NotifyPendingBoot(ctx, key)
}

func (r *boots) TakePending(ctx context.Context) (timeline.PendingBoot, bool, error) {
	b, err := r.q.TakePendingBoot(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return timeline.PendingBoot{}, false, nil
		}
		return timeline.PendingBoot{}, false, err
	}
	return timeline.PendingBoot{BootKey: timeline.BootKey{DongleID: b.DongleID, Generation: b.BootGeneration, BootID: b.BootID},
		FirstSeq: b.FirstSeq, FirstTMs: b.FirstTMs, LastTMs: b.LastTMs, MaxOffsetMs: b.MaxOffsetMs, AMs: b.AMs, B: b.B,
		TimeSource: b.TimeSource}, true, nil
}

func (r *boots) ListAnchors(ctx context.Context, k timeline.BootKey) ([]timeline.Anchor, error) {
	rows, err := r.q.ListBootAnchors(ctx, k.DongleID, k.Generation, k.BootID)
	if err != nil {
		return nil, err
	}
	out := make([]timeline.Anchor, len(rows))
	for i, a := range rows {
		out[i] = timeline.Anchor{TMs: a.TMs, S1Ms: epochMs(a.S1), S2Ms: epochMs(a.S2)}
	}
	return out, nil
}

func epochMs(t time.Time) float64 { return float64(t.UnixMicro()) / 1000 }

func (r *boots) PreviousAnchored(ctx context.Context, dongleID int32, firstSeq int64) (*timeline.NeighborReference, error) {
	x, err := r.q.PreviousAnchoredBoot(ctx, dongleID, &firstSeq)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return neighborFrom(x.AMs, x.B, x.BaseUncertaintyMs, x.FirstTMs, x.LastTMs), nil
}

func (r *boots) NextAnchored(ctx context.Context, dongleID int32, firstSeq int64) (*timeline.NeighborReference, error) {
	x, err := r.q.NextAnchoredBoot(ctx, dongleID, &firstSeq)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return neighborFrom(x.AMs, x.B, x.BaseUncertaintyMs, x.FirstTMs, x.LastTMs), nil
}

func neighborFrom(a, b *float64, inc *int32, first, last *int64) *timeline.NeighborReference {
	if a == nil || b == nil || inc == nil || first == nil || last == nil {
		return nil
	}
	return &timeline.NeighborReference{AMs: *a, B: *b, BaseUncertaintyMs: int(*inc), FirstTMs: *first, LastTMs: *last}
}

func (r *boots) UpdateTime(ctx context.Context, k timeline.BootKey, est *timeline.Estimate, now time.Time) error {
	if est == nil {
		return r.q.UpdateBootTime(ctx, dbgen.UpdateBootTimeParams{Now: &now, DongleID: k.DongleID, BootGeneration: k.Generation, BootID: k.BootID})
	}
	ia, ib := est.AMs, est.B
	inc := int32(est.BaseUncertaintyMs)
	o := est.Source
	return r.q.UpdateBootTime(ctx, dbgen.UpdateBootTimeParams{AMs: &ia, B: &ib, BaseUncertaintyMs: &inc,
		TimeSource: &o, Now: &now, DongleID: k.DongleID, BootGeneration: k.Generation, BootID: k.BootID})
}

func (r *boots) MarkAnchorlessNeighbors(ctx context.Context, dongleID int32, firstSeq int64, now time.Time) (int64, error) {
	return r.q.MarkAnchorlessNeighbors(ctx, now, dongleID, &firstSeq)
}

func (r *boots) MarkGold(ctx context.Context, dongleID int32, fromSeq, toSeq int64) error {
	return r.q.MarkGoldBoots(ctx, dongleID, &toSeq, &fromSeq)
}

func (r *boots) CountPending(ctx context.Context) (int64, error) {
	return r.q.CountPendingBoots(ctx)
}
