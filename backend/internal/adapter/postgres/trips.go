package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"tccelta/backend/internal/database/dbgen"
	"tccelta/backend/internal/domain/trip"
	"tccelta/backend/internal/domain/tripanalysis"
	"tccelta/backend/internal/port"
)

// trips implementa port.Trips. pool, se houver, serve só a Analysis, que abre a própria transação; dentro de uma transação
// (pool nulo) a leitura roda na transação corrente.
type trips struct {
	q    dbgen.Querier
	pool *pgxpool.Pool
}

// TripsWithPool devolve o repositório de viagens para uso fora de transação: a Analysis abre uma transação REPEATABLE READ
// somente leitura no pool.
func TripsWithPool(pool *pgxpool.Pool) port.Trips { return &trips{q: dbgen.New(pool), pool: pool} }

func tripFrom(g dbgen.GoldTrip) trip.Trip {
	return trip.Trip{ID: g.TripID, Category: g.TripCategory, DriverName: g.DriverName, Fuel: g.Fuel, DongleID: g.DongleID,
		EntryType: g.EntryType, BootGeneration: g.BootGeneration, BootID: g.BootID, IntervalStart: g.IntervalStart,
		IntervalEnd: g.IntervalEnd, Status: g.Status, TargetSeq: g.TargetSeq, FirstSeq: g.FirstSeq, LastSeq: g.LastSeq,
		CalculatedThroughSeq: g.CalculatedThroughSeq, Provisional: g.Provisional, RecalcPending: g.RecalcPending,
		CalculationVersion: g.CalculationVersion, CreatedAt: g.CreatedAt, CalculatedAt: g.CalculatedAt}
}

func (r *trips) ListWaiting(ctx context.Context) ([]trip.Trip, error) {
	rows, err := r.q.ListWaitingTrips(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]trip.Trip, len(rows))
	for i, g := range rows {
		out[i] = tripFrom(g)
	}
	return out, nil
}

func (r *trips) MarkWaiting(ctx context.Context, id uuid.UUID) error {
	return r.q.MarkTripWaiting(ctx, id)
}

func (r *trips) ResolveGeneration(ctx context.Context, id uuid.UUID, dongleID, bootID int32) (int32, bool, error) {
	g, err := r.q.ResolveTripGeneration(ctx, id, dongleID, bootID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil || g == nil {
		return 0, false, err
	}
	return *g, true, nil
}

func (r *trips) Take(ctx context.Context, id uuid.UUID) (trip.Trip, bool, error) {
	g, err := r.q.TakeTrip(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return trip.Trip{}, false, nil
	}
	if err != nil {
		return trip.Trip{}, false, err
	}
	return tripFrom(g), true, nil
}

func (r *trips) TakeRecalc(ctx context.Context) (trip.Trip, bool, error) {
	g, err := r.q.TakeRecalcTrip(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return trip.Trip{}, false, nil
	}
	if err != nil {
		return trip.Trip{}, false, err
	}
	return tripFrom(g), true, nil
}

// Save troca as linhas filhas e atualiza a viagem; o chamador abre a transação.
func (r *trips) Save(ctx context.Context, id uuid.UUID, c trip.Calculation) error {
	for _, del := range []func(context.Context, uuid.UUID) error{r.q.DeleteTripParts, r.q.DeleteTripSummary, r.q.DeleteTripSpeedBands,
		r.q.DeleteTripEvents, r.q.DeleteTripStops, r.q.DeleteTripGears, r.q.DeleteTripGearShifts, r.q.DeleteTripSeries} {
		if err := del(ctx, id); err != nil {
			return err
		}
	}
	res := c.Result
	if err := r.insertRows(ctx, id, res); err != nil {
		return err
	}
	sum, err := summaryParams(id, res)
	if err != nil {
		return err
	}
	if err := r.q.InsertTripSummary(ctx, sum); err != nil {
		return err
	}
	ser, err := seriesParams(id, res.Series)
	if err != nil {
		return err
	}
	if err := r.q.InsertTripSeries(ctx, ser); err != nil {
		return err
	}
	version, at, through := tripanalysis.CalculationVersion, c.CalculatedAt, c.CalculatedThroughSeq
	return r.q.SaveTripResult(ctx, dbgen.SaveTripResultParams{CalculationVersion: &version, CalculatedAt: &at, FirstSeq: c.FirstSeq,
		LastSeq: c.LastSeq, CalculatedThroughSeq: &through, Provisional: c.Provisional, TripID: id})
}

func (r *trips) insertRows(ctx context.Context, id uuid.UUID, res tripanalysis.Result) error {
	for i, p := range res.Parts {
		x := dbgen.InsertTripPartParams{TripID: id, PartNo: int16(i + 1), BootGeneration: p.BootGeneration, BootID: p.BootID,
			DataStartTMs: p.DataStartTMs, DataEndTMs: p.DataEndTMs, StartTMs: p.StartTMs, EndTMs: p.EndTMs}
		if p.Clock != nil {
			u := p.Clock.UncertaintyMs
			x.ClockAMs, x.ClockB, x.ClockUncertaintyMs = &p.Clock.AMs, &p.Clock.B, &u
		}
		if err := r.q.InsertTripPart(ctx, x); err != nil {
			return err
		}
	}
	for _, b := range res.SpeedBands {
		if err := r.q.InsertTripSpeedBand(ctx, dbgen.InsertTripSpeedBandParams{TripID: id, BandID: b.ID, FromKmh: b.FromKmh,
			ToKmh: b.ToKmh, TimeS: b.TimeS, DistanceKm: b.DistanceKm, DistancePct: b.DistancePct}); err != nil {
			return err
		}
	}
	for i, e := range res.Events {
		peak := e.Peak
		if err := r.q.InsertTripEvent(ctx, dbgen.InsertTripEventParams{TripID: id, EventNo: int32(i + 1), Type: e.Type,
			StartTMs: e.StartTMs, DurationS: e.DurationS, Peak: &peak, PeakUnit: e.PeakUnit, StartSpeedKmh: e.StartSpeedKmh,
			EndSpeedKmh: e.EndSpeedKmh, Lat: e.Lat, Lon: e.Lon}); err != nil {
			return err
		}
	}
	for i, s := range res.Stops {
		if err := r.q.InsertTripStop(ctx, dbgen.InsertTripStopParams{TripID: id, StopNo: int32(i + 1), StartTMs: s.StartTMs,
			DurationS: s.DurationS, EngineOn: s.EngineOn, Lat: s.Lat, Lon: s.Lon}); err != nil {
			return err
		}
	}
	for _, g := range res.Gears {
		if err := r.q.InsertTripGear(ctx, dbgen.InsertTripGearParams{TripID: id, Gear: int16(g.Gear), RpmPerKmh: g.RPMPerKmh,
			TimeS: g.TimeS, TimePct: g.TimePct}); err != nil {
			return err
		}
	}
	for i, s := range res.GearShifts {
		if err := r.q.InsertTripGearShift(ctx, dbgen.InsertTripGearShiftParams{TripID: id, ShiftNo: int32(i + 1), TMs: s.TMs,
			Direction: s.Direction, FromGear: int16(s.From), ToGear: int16(s.To), RpmBefore: s.RPMBefore, RpmAfter: s.RPMAfter,
			SpeedKmh: s.SpeedKmh}); err != nil {
			return err
		}
	}
	return nil
}

func seriesParams(id uuid.UUID, s tripanalysis.SeriesData) (dbgen.InsertTripSeriesParams, error) {
	p := dbgen.InsertTripSeriesParams{TripID: id, StepMs: s.StepMs, ExposureStepMs: s.ExposureStepMs}
	var err error
	for dst, v := range map[*[]byte]any{&p.Series: s.Series, &p.SlowSeries: s.Slow, &p.Route: s.Route,
		&p.RpmDistribution: s.RPMDistribution, &p.PedalDistribution: s.PedalDistribution, &p.RpmPedalMatrix: s.RPMPedal} {
		if *dst, err = json.Marshal(v); err != nil {
			return p, err
		}
	}
	return p, nil
}

func summaryParams(id uuid.UUID, res tripanalysis.Result) (dbgen.InsertTripSummaryParams, error) {
	s := res.Summary
	warnings, err := json.Marshal(res.Warnings)
	if err != nil {
		return dbgen.InsertTripSummaryParams{}, err
	}
	params, err := json.Marshal(res.Params)
	if err != nil {
		return dbgen.InsertTripSummaryParams{}, err
	}
	periods, err := json.Marshal(s.PIDPeriodsMs)
	if err != nil {
		return dbgen.InsertTripSummaryParams{}, err
	}
	nReadings := s.NReadings
	return dbgen.InsertTripSummaryParams{
		TripID: id, StartTMs: s.StartTMs, EndTMs: s.EndTMs, StartedAt: s.StartedAt, EndedAt: s.EndedAt,
		ClockOffsetMs: s.ClockOffsetMs, ClockUncertaintyMs: s.ClockUncertaintyMs, Warnings: warnings, Parameters: params,
		DistanceKm: s.DistanceKm, DurationS: s.DurationS, NoDataS: s.NoDataS, MovingS: s.MovingS, StoppedS: s.StoppedS,
		AvgSpeedKmh: s.AvgSpeedKmh, MaxSpeedKmh: s.MaxSpeedKmh, EngineOnS: s.EngineOnS, IdleS: s.IdleS, IdlePct: s.IdlePct,
		FuelL: s.FuelL, KmPerL: s.KmPerL, IntakeAirG: s.IntakeAirG, NHardAccel: s.NHardAccel, NHardBrake: s.NHardBrake,
		HardAccelPer100km: s.HardAccelPer100Km, HardBrakePer100km: s.HardBrakePer100Km,
		PedalMeanPct: s.PedalMeanPct, PedalHighS: s.PedalHighS, PedalRateP95PctS: s.PedalRateP95PctS,
		EngineBrakePct: s.EngineBrakePct, AccelRmsMs2: s.AccelRMSMS2, CruiseS: s.CruiseS,
		WarmupS: s.WarmupS, MaxCoolantC: s.MaxCoolantC, ColdLoadS: s.ColdLoadS,
		IdleRpm: s.IdleRPM, IdleRpmOrigin: s.IdleRPMOrigin, IdleMarginRpm: s.IdleMarginRPM, NeutralClutchS: s.NeutralClutchS,
		UpshiftRpmMedian: s.UpshiftRPMMedian, UpshiftRpmP25: s.UpshiftRPMP25, UpshiftRpmP75: s.UpshiftRPMP75, UpshiftRpmN: s.UpshiftN,
		DownshiftRpmMedian: s.DownshiftRPMMedian, DownshiftRpmN: s.DownshiftN,
		NReadings: &nReadings, ReadingsPerS: s.ReadingsPerS, PidPeriodsMs: periods, FuelCoveragePct: s.FuelCoveragePct,
		GpsPeriodS: s.GPSPeriodS, GpsCoveragePct: s.GPSCoveragePct, GpsNGaps: s.GPSNGaps, GpsMaxGapS: s.GPSMaxGapS,
		GpsDistanceKm: s.GPSDistanceKm, GpsDistanceDiffPct: s.GPSDistanceDiffPct,
	}, nil
}

func (r *trips) Fail(ctx context.Context, id uuid.UUID) error { return r.q.FailTrip(ctx, id) }

func (r *trips) ListProvisional(ctx context.Context) ([]trip.Provisional, error) {
	rows, err := r.q.ListProvisionalTrips(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]trip.Provisional, len(rows))
	for i, x := range rows {
		g := dbgen.GoldTrip{TripID: x.TripID, TripCategory: x.TripCategory, DriverName: x.DriverName, DriverNameNormalized: x.DriverNameNormalized,
			Fuel: x.Fuel, DongleID: x.DongleID, EntryType: x.EntryType, BootGeneration: x.BootGeneration, BootID: x.BootID,
			IntervalStart: x.IntervalStart, IntervalEnd: x.IntervalEnd, Status: x.Status, TargetSeq: x.TargetSeq, FirstSeq: x.FirstSeq,
			LastSeq: x.LastSeq, CalculatedThroughSeq: x.CalculatedThroughSeq, Provisional: x.Provisional, RecalcPending: x.RecalcPending,
			CalculationVersion: x.CalculationVersion, CreatedAt: x.CreatedAt, CalculatedAt: x.CalculatedAt, ExcludedAt: x.ExcludedAt}
		out[i] = trip.Provisional{Trip: tripFrom(g), NoTimeBoots: noTimeBoots(x.SummaryWarnings)}
	}
	return out, nil
}

// noTimeBoots lê os boots dos avisos boot_sem_hora guardados em trip_summary.warnings.
func noTimeBoots(raw []byte) []trip.BootRef {
	var ws []struct {
		Code   string `json:"codigo"`
		Detail struct {
			Generation *int32 `json:"boot_generation"`
			BootID     *int32 `json:"boot_id"`
		} `json:"detalhe"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &ws) != nil {
		return nil
	}
	var out []trip.BootRef
	for _, w := range ws {
		if w.Code == tripanalysis.WarnBootNoTime && w.Detail.Generation != nil && w.Detail.BootID != nil {
			out = append(out, trip.BootRef{Generation: *w.Detail.Generation, BootID: *w.Detail.BootID})
		}
	}
	return out
}

func (r *trips) MarkRecalc(ctx context.Context, ids []uuid.UUID) error {
	return r.q.MarkTripsRecalcByID(ctx, ids)
}

func (r *trips) ClearProvisional(ctx context.Context, id uuid.UUID) error {
	return r.q.ClearTripProvisional(ctx, id)
}

func (r *trips) MarkRecalcByRange(ctx context.Context, dongleID int32, fromSeq, toSeq int64) error {
	return r.q.MarkTripsRecalc(ctx, dongleID, &toSeq, &fromSeq)
}

func (r *trips) Notify(ctx context.Context) error { return r.q.NotifyTripPending(ctx) }

func (r *trips) CountQueue(ctx context.Context) (trip.Queue, error) {
	q, err := r.q.CountTripQueue(ctx)
	return trip.Queue{Waiting: q.Waiting, Recalc: q.Recalc}, err
}

// driverNameUnique é o índice único de motorista por categoria (gold.trip).
const driverNameUnique = "trip_driver_name_unique"

func (r *trips) Insert(ctx context.Context, d trip.Draft) (trip.Trip, error) {
	g, err := r.q.InsertTrip(ctx, dbgen.InsertTripParams{TripCategory: d.Category, DriverName: d.DriverName,
		DriverNameNormalized: d.DriverNameNormalized, Fuel: d.Fuel, DongleID: d.DongleID, EntryType: d.EntryType,
		BootGeneration: d.BootGeneration, BootID: d.BootID, IntervalStart: d.IntervalStart, IntervalEnd: d.IntervalEnd,
		TargetSeq: d.TargetSeq})
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" && pe.ConstraintName == driverNameUnique {
		return trip.Trip{}, trip.ErrDriverRepeated
	}
	if err != nil {
		return trip.Trip{}, err
	}
	return tripFrom(g), nil
}

func (r *trips) NotifyTrip(ctx context.Context, id uuid.UUID) error {
	return r.q.NotifyTripCreated(ctx, id.String())
}

func (r *trips) Find(ctx context.Context, id uuid.UUID) (trip.Trip, error) {
	g, err := r.q.FindTrip(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return trip.Trip{}, trip.ErrNotFound
	}
	if err != nil {
		return trip.Trip{}, err
	}
	return tripFrom(g), nil
}

func (r *trips) Analysis(ctx context.Context, id uuid.UUID) (trip.Analysis, error) {
	if r.pool == nil {
		return readAnalysis(ctx, r.q, id)
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return trip.Analysis{}, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	a, err := readAnalysis(ctx, dbgen.New(tx), id)
	if err != nil {
		return trip.Analysis{}, err
	}
	return a, tx.Commit(ctx)
}

// readAnalysis lê a viagem e, se ela estiver pronta, as tabelas filhas.
func readAnalysis(ctx context.Context, q dbgen.Querier, id uuid.UUID) (trip.Analysis, error) {
	g, err := q.FindTrip(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return trip.Analysis{}, trip.ErrNotFound
	}
	if err != nil {
		return trip.Analysis{}, err
	}
	a := trip.Analysis{Trip: tripFrom(g)}
	if g.Status != trip.StatusReady {
		return a, nil
	}
	sum, err := q.GetTripSummary(ctx, id)
	if err != nil {
		return trip.Analysis{}, err
	}
	ser, err := q.GetTripSeries(ctx, id)
	if err != nil {
		return trip.Analysis{}, err
	}
	a.Summary, a.Warnings, a.Params = summaryFrom(sum), jsonList(sum.Warnings), jsonObject(sum.Parameters)
	a.Series = trip.AnalysisSeries{StepMs: ser.StepMs, ExposureStepMs: ser.ExposureStepMs, Series: ser.Series, Slow: ser.SlowSeries,
		Route: routeOrNil(ser.Route), RPMDistribution: jsonList(ser.RpmDistribution), PedalDistribution: jsonList(ser.PedalDistribution),
		RPMPedalGrid: ser.RpmPedalMatrix}
	return a, fillRows(ctx, q, id, &a)
}

// fillRows lê as tabelas de linhas da viagem; as listas saem vazias (nunca nulas) e na ordem das consultas.
func fillRows(ctx context.Context, q dbgen.Querier, id uuid.UUID, a *trip.Analysis) error {
	parts, err := q.ListTripParts(ctx, id)
	if err != nil {
		return err
	}
	a.Parts = make([]tripanalysis.PartResult, len(parts))
	for i, p := range parts {
		x := tripanalysis.PartResult{BootGeneration: p.BootGeneration, BootID: p.BootID, DataStartTMs: p.DataStartTMs,
			DataEndTMs: p.DataEndTMs, StartTMs: p.StartTMs, EndTMs: p.EndTMs}
		if p.ClockAMs != nil && p.ClockB != nil && p.ClockUncertaintyMs != nil {
			x.Clock = &tripanalysis.Clock{AMs: *p.ClockAMs, B: *p.ClockB, UncertaintyMs: *p.ClockUncertaintyMs}
		}
		a.Parts[i] = x
	}
	bands, err := q.ListTripSpeedBands(ctx, id)
	if err != nil {
		return err
	}
	a.SpeedBands = make([]tripanalysis.SpeedBand, len(bands))
	for i, b := range bands {
		a.SpeedBands[i] = tripanalysis.SpeedBand{ID: b.BandID, FromKmh: b.FromKmh, ToKmh: finite(b.ToKmh), TimeS: b.TimeS,
			DistanceKm: b.DistanceKm, DistancePct: finite(b.DistancePct)}
	}
	events, err := q.ListTripEvents(ctx, id)
	if err != nil {
		return err
	}
	a.Events = make([]tripanalysis.Event, len(events))
	for i, e := range events {
		x := tripanalysis.Event{Type: e.Type, StartTMs: e.StartTMs, DurationS: e.DurationS, PeakUnit: e.PeakUnit,
			StartSpeedKmh: finite(e.StartSpeedKmh), EndSpeedKmh: finite(e.EndSpeedKmh), Lat: finite(e.Lat), Lon: finite(e.Lon)}
		if e.Peak != nil {
			x.Peak = *e.Peak
		}
		a.Events[i] = x
	}
	stops, err := q.ListTripStops(ctx, id)
	if err != nil {
		return err
	}
	a.Stops = make([]tripanalysis.Stop, len(stops))
	for i, s := range stops {
		a.Stops[i] = tripanalysis.Stop{StartTMs: s.StartTMs, DurationS: s.DurationS, EngineOn: s.EngineOn, Lat: finite(s.Lat), Lon: finite(s.Lon)}
	}
	gears, err := q.ListTripGears(ctx, id)
	if err != nil {
		return err
	}
	a.Gears = make([]tripanalysis.Gear, len(gears))
	for i, g := range gears {
		a.Gears[i] = tripanalysis.Gear{Gear: int(g.Gear), RPMPerKmh: g.RpmPerKmh, TimeS: g.TimeS, TimePct: g.TimePct}
	}
	shifts, err := q.ListTripGearShifts(ctx, id)
	if err != nil {
		return err
	}
	a.GearShifts = make([]tripanalysis.GearShift, len(shifts))
	for i, s := range shifts {
		a.GearShifts[i] = tripanalysis.GearShift{TMs: s.TMs, Direction: s.Direction, From: int(s.FromGear), To: int(s.ToGear),
			RPMBefore: s.RpmBefore, RPMAfter: s.RpmAfter, SpeedKmh: s.SpeedKmh}
	}
	return nil
}

// finite troca NaN e infinito por nil: encoding/json falha neles, e o contrato diz que ausência é null.
func finite(p *float64) *float64 {
	if p == nil || math.IsNaN(*p) || math.IsInf(*p, 0) {
		return nil
	}
	return p
}

// jsonList devolve a lista jsonb ou [] se ela for nula/vazia (o contrato nunca tem lista nula).
func jsonList(raw []byte) trip.RawJSON {
	if s := strings.TrimSpace(string(raw)); s == "" || s == "null" {
		return trip.RawJSON("[]")
	}
	return raw
}

func jsonObject(raw []byte) trip.RawJSON {
	if s := strings.TrimSpace(string(raw)); s == "" || s == "null" {
		return trip.RawJSON("{}")
	}
	return raw
}

// routeOrNil devolve nil (rota null no contrato) quando o worker não gravou nenhum ponto de GPS.
func routeOrNil(raw []byte) trip.RawJSON {
	var r struct {
		TMs []json.RawMessage `json:"t_ms"`
	}
	if json.Unmarshal(raw, &r) != nil || len(r.TMs) == 0 {
		return nil
	}
	return raw
}

func summaryFrom(s dbgen.GoldTripSummary) tripanalysis.Summary {
	f := finite
	out := tripanalysis.Summary{
		StartTMs: s.StartTMs, EndTMs: s.EndTMs, StartedAt: s.StartedAt, EndedAt: s.EndedAt, ClockOffsetMs: s.ClockOffsetMs,
		ClockUncertaintyMs: s.ClockUncertaintyMs,
		DistanceKm:         f(s.DistanceKm), DurationS: f(s.DurationS), NoDataS: f(s.NoDataS), MovingS: f(s.MovingS), StoppedS: f(s.StoppedS),
		AvgSpeedKmh: f(s.AvgSpeedKmh), MaxSpeedKmh: f(s.MaxSpeedKmh), EngineOnS: f(s.EngineOnS), IdleS: f(s.IdleS), IdlePct: f(s.IdlePct),
		FuelL: f(s.FuelL), KmPerL: f(s.KmPerL), IntakeAirG: f(s.IntakeAirG), NHardAccel: s.NHardAccel, NHardBrake: s.NHardBrake,
		HardAccelPer100Km: f(s.HardAccelPer100km), HardBrakePer100Km: f(s.HardBrakePer100km),
		PedalMeanPct: f(s.PedalMeanPct), PedalHighS: f(s.PedalHighS), PedalRateP95PctS: f(s.PedalRateP95PctS),
		EngineBrakePct: f(s.EngineBrakePct), AccelRMSMS2: f(s.AccelRmsMs2), CruiseS: f(s.CruiseS),
		WarmupS: f(s.WarmupS), MaxCoolantC: f(s.MaxCoolantC), ColdLoadS: f(s.ColdLoadS),
		IdleRPM: f(s.IdleRpm), IdleRPMOrigin: s.IdleRpmOrigin, IdleMarginRPM: f(s.IdleMarginRpm), NeutralClutchS: f(s.NeutralClutchS),
		UpshiftRPMMedian: f(s.UpshiftRpmMedian), UpshiftRPMP25: f(s.UpshiftRpmP25), UpshiftRPMP75: f(s.UpshiftRpmP75), UpshiftN: s.UpshiftRpmN,
		DownshiftRPMMedian: f(s.DownshiftRpmMedian), DownshiftN: s.DownshiftRpmN,
		ReadingsPerS: f(s.ReadingsPerS), FuelCoveragePct: f(s.FuelCoveragePct),
		GPSPeriodS: f(s.GpsPeriodS), GPSCoveragePct: f(s.GpsCoveragePct), GPSNGaps: s.GpsNGaps, GPSMaxGapS: f(s.GpsMaxGapS),
		GPSDistanceKm: f(s.GpsDistanceKm), GPSDistanceDiffPct: f(s.GpsDistanceDiffPct),
	}
	if s.NReadings != nil {
		out.NReadings = *s.NReadings
	}
	_ = json.Unmarshal(s.PidPeriodsMs, &out.PIDPeriodsMs)
	for k, v := range out.PIDPeriodsMs {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			delete(out.PIDPeriodsMs, k)
		}
	}
	return out
}
