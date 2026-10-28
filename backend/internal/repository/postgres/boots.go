package postgres

import (
	"context"
	"time"

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
