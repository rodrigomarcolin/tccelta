package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"tccelta/backend/internal/database/dbgen"
	"tccelta/backend/internal/domain/trip"
)

type tripSources struct{ q dbgen.Querier }

func bootFrom(generation, bootID int32, firstSeq, lastSeq, firstT, lastT *int64, maxOffset, a, b *float64, unc *int32, src *string) trip.Boot {
	return trip.Boot{Generation: generation, BootID: bootID, FirstSeq: firstSeq, LastSeq: lastSeq, FirstTMs: firstT, LastTMs: lastT,
		MaxOffsetMs: maxOffset, AMs: a, B: b, BaseUncertaintyMs: unc, TimeSource: src}
}

func (r *tripSources) FindBoot(ctx context.Context, dongleID, generation, bootID int32) (trip.Boot, bool, error) {
	x, err := r.q.FindBootForTrip(ctx, dongleID, generation, bootID)
	if errors.Is(err, pgx.ErrNoRows) {
		return trip.Boot{}, false, nil
	}
	if err != nil {
		return trip.Boot{}, false, err
	}
	return bootFrom(x.BootGeneration, x.BootID, x.FirstSeq, x.LastSeq, x.FirstTMs, x.LastTMs, x.MaxOffsetMs, x.AMs, x.B, x.BaseUncertaintyMs, x.TimeSource), true, nil
}

func (r *tripSources) IntervalBoots(ctx context.Context, dongleID int32, fromMs, toMs float64) ([]trip.Boot, error) {
	rows, err := r.q.ListIntervalBoots(ctx, dongleID, fromMs, toMs)
	if err != nil {
		return nil, err
	}
	out := make([]trip.Boot, len(rows))
	for i, x := range rows {
		out[i] = bootFrom(x.BootGeneration, x.BootID, x.FirstSeq, x.LastSeq, x.FirstTMs, x.LastTMs, x.MaxOffsetMs, x.AMs, x.B, x.BaseUncertaintyMs, x.TimeSource)
	}
	return out, nil
}

func (r *tripSources) UnconvertedBoots(ctx context.Context, dongleID int32, t0Ms, t1Ms float64) ([]trip.Boot, error) {
	rows, err := r.q.ListUnconvertedBoots(ctx, dongleID, t0Ms, t1Ms)
	if err != nil {
		return nil, err
	}
	out := make([]trip.Boot, len(rows))
	for i, x := range rows {
		out[i] = bootFrom(x.BootGeneration, x.BootID, x.FirstSeq, x.LastSeq, x.FirstTMs, x.LastTMs, x.MaxOffsetMs, x.AMs, x.B, x.BaseUncertaintyMs, x.TimeSource)
	}
	return out, nil
}

func (r *tripSources) ReceivedRange(ctx context.Context, dongleID int32, b trip.Boot) (time.Time, time.Time, bool, error) {
	if b.FirstSeq == nil || b.LastSeq == nil {
		return time.Time{}, time.Time{}, false, nil
	}
	first, err := r.q.BootFirstReceived(ctx, dbgen.BootFirstReceivedParams{DongleID: dongleID, FirstSeq: *b.FirstSeq, LastSeq: *b.LastSeq,
		BootID: b.BootID, BootGeneration: b.Generation})
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, time.Time{}, false, err
	}
	last, err := r.q.BootLastReceived(ctx, dbgen.BootLastReceivedParams{DongleID: dongleID, FirstSeq: *b.FirstSeq, LastSeq: *b.LastSeq,
		BootID: b.BootID, BootGeneration: b.Generation})
	if err != nil {
		return time.Time{}, time.Time{}, false, err
	}
	return first, last, true, nil
}

func (r *tripSources) LoadReadings(ctx context.Context, q trip.ReadingQuery) ([]trip.ReadingRow, error) {
	rows, err := r.q.LoadTripReadings(ctx, dbgen.LoadTripReadingsParams{DongleID: q.DongleID, BootGeneration: q.Generation, BootID: q.BootID,
		Pids: q.PIDs, ReceivedFrom: q.ReceivedFrom, ReceivedTo: q.ReceivedTo, TFrom: q.TFrom, TTo: q.TTo})
	if err != nil {
		return nil, err
	}
	out := make([]trip.ReadingRow, 0, len(rows))
	for _, x := range rows {
		if x.Value != nil {
			out = append(out, trip.ReadingRow{PID: x.Pid, TMs: x.TMs, Value: float64(*x.Value)})
		}
	}
	return out, nil
}

func (r *tripSources) LoadPositions(ctx context.Context, q trip.ReadingQuery) ([]trip.PositionRow, error) {
	rows, err := r.q.LoadTripPositions(ctx, dbgen.LoadTripPositionsParams{DongleID: q.DongleID, BootGeneration: q.Generation, BootID: q.BootID,
		ReceivedFrom: q.ReceivedFrom, ReceivedTo: q.ReceivedTo, TFrom: q.TFrom, TTo: q.TTo})
	if err != nil {
		return nil, err
	}
	out := make([]trip.PositionRow, len(rows))
	for i, x := range rows {
		out[i] = trip.PositionRow{TMs: x.TMs, Lat: x.Lat, Lon: x.Lon}
	}
	return out, nil
}

func (r *tripSources) NextBootStart(ctx context.Context, dongleID int32, afterSeq int64) (int64, bool, error) {
	s, err := r.q.NextBootFirstSeq(ctx, dongleID, &afterSeq)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && s == nil) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return *s, true, nil
}

func (r *tripSources) MaxBlockTime(ctx context.Context, dongleID int32, fromSeq, toSeq int64) (float64, bool, error) {
	ms, err := r.q.MaxConvertedBlockTime(ctx, dongleID, fromSeq, toSeq)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return ms, true, nil
}
