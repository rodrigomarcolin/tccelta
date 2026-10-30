package postgres

import (
	"context"

	"tccelta/backend/internal/database/dbgen"
	"tccelta/backend/internal/domain/dongle"
	"tccelta/backend/internal/domain/handshake"
	"tccelta/backend/internal/domain/ingestion"
)

type states struct{ q dbgen.Querier }

func (r *states) Read(ctx context.Context, dongleID int32) (dongle.State, error) {
	e, err := r.q.ReadState(ctx, dongleID)
	if err != nil {
		return dongle.State{}, err
	}
	return dongle.State{ContiguousSeq: e.ContiguousSeq, MaxSeenSeq: e.MaxSeenSeq, HeadSeq: e.HeadSeq, CConf: e.CConf,
		BootGeneration: e.BootGeneration, LastBootID: e.LastBootID}, nil
}

func (r *states) ReadSnapshot(ctx context.Context, dongleID int32) (dongle.State, error) {
	e, err := r.q.ReadStateSnapshot(ctx, dongleID)
	if err != nil {
		return dongle.State{}, err
	}
	return dongle.State{ContiguousSeq: e.ContiguousSeq, CConf: e.CConf, LastAck: e.LastAck}, nil
}

func (r *states) ReadLastAck(ctx context.Context, dongleID int32) ([]byte, error) {
	return r.q.ReadLastAck(ctx, dongleID)
}

func (r *states) UpdateHandshake(ctx context.Context, a handshake.Update) error {
	tail, s2, now := a.TailSeq, a.S2, a.Now
	return r.q.UpdateHandshakeState(ctx, dbgen.UpdateHandshakeStateParams{
		ContiguousSeq: a.ContiguousSeq, HeadSeq: a.HeadSeq, TailSeq: &tail, S2: &s2, CConf: a.CConf, LastAck: a.LastAck,
		Now: &now, DongleID: a.DongleID})
}

func (r *states) UpdateIngestion(ctx context.Context, a ingestion.StateUpdate) error {
	now := a.Now
	return r.q.UpdateIngestionState(ctx, dbgen.UpdateIngestionStateParams{
		ContiguousSeq: a.ContiguousSeq, MaxSeen: a.MaxSeen, LastAck: a.LastAck, Now: &now,
		BootGeneration: a.BootGeneration, LastBootID: a.LastBootID, DongleID: a.DongleID})
}
