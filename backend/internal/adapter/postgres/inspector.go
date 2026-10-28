package postgres

import (
	"context"

	"tccelta/backend/internal/database/dbgen"
	"tccelta/backend/internal/domain/protocol"
	"tccelta/backend/internal/port"
)

type inspector struct{ q dbgen.Querier }

// Inspector monta a leitura de verificação do simulador sobre q (papel com SELECT).
func Inspector(q dbgen.Querier) port.Inspector { return &inspector{q} }

func (r *inspector) DongleState(ctx context.Context, dev protocol.DevID) (port.InspectedState, error) {
	e, err := r.q.VerifyDongleState(ctx, dev[:])
	if err != nil {
		return port.InspectedState{}, err
	}
	return port.InspectedState{DongleID: e.DongleID, ContiguousSeq: e.ContiguousSeq, HeadSeq: e.HeadSeq, CConf: e.CConf,
		BootGeneration: e.BootGeneration, LastBootID: e.LastBootID}, nil
}

func (r *inspector) Gaps(ctx context.Context, dongleID int32) ([]port.InspectedGap, error) {
	rows, err := r.q.VerifyListGaps(ctx, dongleID)
	if err != nil {
		return nil, err
	}
	out := make([]port.InspectedGap, len(rows))
	for i, x := range rows {
		out[i] = port.InspectedGap{FromSeq: x.FromSeq, ToSeq: x.ToSeq, Cause: x.Cause}
	}
	return out, nil
}

func (r *inspector) Boots(ctx context.Context, dongleID int32) ([]port.InspectedBoot, error) {
	rows, err := r.q.VerifyListBoots(ctx, dongleID)
	if err != nil {
		return nil, err
	}
	out := make([]port.InspectedBoot, len(rows))
	for i, x := range rows {
		out[i] = port.InspectedBoot{BootGeneration: x.BootGeneration, BootID: x.BootID, TimeSource: x.TimeSource, AMs: x.AMs, B: x.B,
			BaseUncertaintyMs: x.BaseUncertaintyMs, TimePending: x.TimePending}
	}
	return out, nil
}

func (r *inspector) CountReadings(ctx context.Context, dongleID int32) (int64, error) {
	return r.q.VerifyCountReadings(ctx, dongleID)
}
func (r *inspector) CountPositions(ctx context.Context, dongleID int32) (int64, error) {
	return r.q.VerifyCountPositions(ctx, dongleID)
}
func (r *inspector) CountRejected(ctx context.Context, dongleID int32) (int64, error) {
	return r.q.VerifyCountRejected(ctx, dongleID)
}
func (r *inspector) CountAnchors(ctx context.Context, dongleID int32) (int64, error) {
	return r.q.VerifyCountAnchors(ctx, dongleID)
}
func (r *inspector) CountAlerts(ctx context.Context, dongleID int32, alertType string) (int64, error) {
	return r.q.VerifyCountAlerts(ctx, dongleID, alertType)
}
func (r *inspector) ReadingTime(ctx context.Context, dongleID int32) (float64, error) {
	return r.q.VerifyReadingTime(ctx, dongleID)
}

var _ port.Inspector = (*inspector)(nil)
