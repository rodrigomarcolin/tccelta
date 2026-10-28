package postgres

import (
	"context"
	"time"

	"tccelta/backend/internal/database/dbgen"
	"tccelta/backend/internal/domain/ingestion"
	"tccelta/backend/internal/domain/pid"
	"tccelta/backend/internal/port"
)

type blocks struct{ q dbgen.Querier }

func (r *blocks) InsertRawBlocks(ctx context.Context, dongleID int32, epoch int64, receivedAt time.Time, bs []ingestion.RawBlock) (int, error) {
	n := len(bs)
	in := dbgen.InsertRawBlocksParams{DongleID: dongleID, Epoch: epoch, ReceivedAt: receivedAt,
		Seqs: make([]int64, n), BootIds: make([]int32, n), Generations: make([]int32, n), T0s: make([]int64, n),
		Markers: make([]string, n), Payloads: make([][]byte, n), Signatures: make([][]byte, n)}
	for i, b := range bs {
		in.Seqs[i], in.BootIds[i], in.Generations[i], in.T0s[i] = b.Seq, b.BootID, b.Generation, b.T0Ms
		in.Markers[i], in.Payloads[i], in.Signatures[i] = b.Marker, b.Payload, b.Signature
	}
	stored, err := r.q.InsertRawBlocks(ctx, in)
	return len(stored), err
}

func (r *blocks) RegisterRejected(ctx context.Context, x ingestion.Rejected) error {
	var det *string
	if x.Detail != "" {
		d := x.Detail
		det = &d
	}
	seq, boot, t0, jti := x.Seq, x.BootID, x.T0Ms, x.TokenJTI
	return r.q.RegisterRejected(ctx, dbgen.RegisterRejectedParams{DongleID: x.DongleID, Seq: &seq, BootID: &boot, T0Ms: &t0,
		Payload: x.Payload, Signature: x.Signature, Reason: string(x.Reason), Detail: det, ContentHash: x.ContentHash,
		TokenJti: &jti, ReceivedAt: x.ReceivedAt})
}

func (r *blocks) CopyReadings(ctx context.Context, dongleID int32, receivedAt time.Time, ls []pid.Reading) error {
	ps := make([]dbgen.CopyReadingsParams, len(ls))
	for i, l := range ls {
		ps[i] = dbgen.CopyReadingsParams{DongleID: dongleID, BootGeneration: l.Generation, BootID: l.BootID, TMs: l.TMs,
			Pid: l.PID, RawValue: l.RawValue, Value: l.Value, BlockSeq: l.BlockSeq, ReceivedAt: receivedAt}
	}
	_, err := r.q.CopyReadings(ctx, ps)
	return err
}

func (r *blocks) CopyPositions(ctx context.Context, dongleID int32, receivedAt time.Time, pos []pid.Position) error {
	ps := make([]dbgen.CopyPositionsParams, len(pos))
	for i, p := range pos {
		ps[i] = dbgen.CopyPositionsParams{DongleID: dongleID, BootGeneration: p.Generation, BootID: p.BootID, TMs: p.TMs,
			Geom: p.Geom.EWKB(), Source: "app", BlockSeq: p.BlockSeq, ReceivedAt: receivedAt}
	}
	_, err := r.q.CopyPositions(ctx, ps)
	return err
}

func (r *blocks) CopyReadingsAt(ctx context.Context, dongleID int32, ls []port.ReadingAt) error {
	ps := make([]dbgen.CopyReadingsParams, len(ls))
	for i, l := range ls {
		ps[i] = dbgen.CopyReadingsParams{DongleID: dongleID, BootGeneration: l.Generation, BootID: l.BootID, TMs: l.TMs,
			Pid: l.PID, RawValue: l.RawValue, Value: l.Value, BlockSeq: l.BlockSeq, ReceivedAt: l.ReceivedAt}
	}
	_, err := r.q.CopyReadings(ctx, ps)
	return err
}

func (r *blocks) CopyPositionsAt(ctx context.Context, dongleID int32, pos []port.PositionAt) error {
	ps := make([]dbgen.CopyPositionsParams, len(pos))
	for i, p := range pos {
		ps[i] = dbgen.CopyPositionsParams{DongleID: dongleID, BootGeneration: p.Generation, BootID: p.BootID, TMs: p.TMs,
			Geom: p.Geom.EWKB(), Source: "app", BlockSeq: p.BlockSeq, ReceivedAt: p.ReceivedAt}
	}
	_, err := r.q.CopyPositions(ctx, ps)
	return err
}

func (r *blocks) ListData(ctx context.Context, dongleID int32, fromSeq, toSeq int64, limit int32) ([]ingestion.StoredBlock, error) {
	rows, err := r.q.ListDataBlocks(ctx, dongleID, fromSeq, toSeq, limit)
	if err != nil {
		return nil, err
	}
	out := make([]ingestion.StoredBlock, len(rows))
	for i, x := range rows {
		out[i] = ingestion.StoredBlock{Seq: x.Seq, Generation: x.BootGeneration, BootID: x.BootID, T0Ms: x.T0Ms,
			Payload: x.Payload, ReceivedAt: x.ReceivedAt}
	}
	return out, nil
}

func (r *blocks) CountReadings(ctx context.Context, dongleID int32, fromSeq, toSeq int64) (int64, error) {
	return r.q.CountReadings(ctx, dongleID, fromSeq, toSeq)
}

func (r *blocks) DeleteReadings(ctx context.Context, dongleID int32, fromSeq, toSeq int64, recvMin, recvMax time.Time) (int64, error) {
	return r.q.DeleteBlockReadings(ctx, dbgen.DeleteBlockReadingsParams{DongleID: dongleID, FromSeq: fromSeq, ToSeq: toSeq,
		ReceivedMin: recvMin, ReceivedMax: recvMax})
}

func (r *blocks) DeletePositions(ctx context.Context, dongleID int32, fromSeq, toSeq int64, recvMin, recvMax time.Time) (int64, error) {
	return r.q.DeleteBlockPositions(ctx, dbgen.DeleteBlockPositionsParams{DongleID: dongleID, FromSeq: fromSeq, ToSeq: toSeq,
		ReceivedMin: recvMin, ReceivedMax: recvMax})
}
