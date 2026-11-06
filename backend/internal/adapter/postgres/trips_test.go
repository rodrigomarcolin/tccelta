package postgres

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"tccelta/backend/internal/database/dbgen"
	"tccelta/backend/internal/domain/dongle"
	"tccelta/backend/internal/domain/trip"
)

// fakeQ implementa só as consultas da API2; as outras entram nil pela interface embutida e quebrariam o teste se fossem usadas.
type fakeQ struct {
	dbgen.Querier
	trip    dbgen.GoldTrip
	tripErr error
	insert  error
	summary dbgen.GoldTripSummary
	series  dbgen.GoldTripSeries
	gen     int32
	genErr  error
	state   error
	notify  string
	calls   []string
}

func (q *fakeQ) FindTrip(context.Context, uuid.UUID) (dbgen.GoldTrip, error) {
	q.calls = append(q.calls, "FindTrip")
	return q.trip, q.tripErr
}

func (q *fakeQ) InsertTrip(_ context.Context, a dbgen.InsertTripParams) (dbgen.GoldTrip, error) {
	g := q.trip
	g.TripCategory, g.DriverName, g.Status = a.TripCategory, a.DriverName, "pending"
	return g, q.insert
}

func (q *fakeQ) NotifyTripCreated(_ context.Context, payload string) error {
	q.notify = payload
	return nil
}

func (q *fakeQ) GetTripSummary(context.Context, uuid.UUID) (dbgen.GoldTripSummary, error) {
	return q.summary, nil
}

func (q *fakeQ) GetTripSeries(context.Context, uuid.UUID) (dbgen.GoldTripSeries, error) {
	return q.series, nil
}

func (q *fakeQ) ListTripParts(context.Context, uuid.UUID) ([]dbgen.GoldTripPart, error) {
	a, b, u := 1.0, 0.001, int32(180)
	return []dbgen.GoldTripPart{{PartNo: 1, BootGeneration: 0, BootID: 8, DataStartTMs: 1, DataEndTMs: 9, ClockAMs: &a, ClockB: &b, ClockUncertaintyMs: &u},
		{PartNo: 2, BootGeneration: 0, BootID: 9}}, nil
}

func (q *fakeQ) ListTripSpeedBands(context.Context, uuid.UUID) ([]dbgen.GoldTripSpeedBand, error) {
	nan := math.NaN()
	return []dbgen.GoldTripSpeedBand{{BandID: "parado", ToKmh: &nan}}, nil
}

func (q *fakeQ) ListTripEvents(context.Context, uuid.UUID) ([]dbgen.GoldTripEvent, error) {
	peak, lat := -6.8, math.Inf(1)
	return []dbgen.GoldTripEvent{{Type: "frenagem_brusca", Peak: &peak, Lat: &lat}, {Type: "marcha_lenta_longa"}}, nil
}

func (q *fakeQ) ListTripStops(context.Context, uuid.UUID) ([]dbgen.GoldTripStop, error) {
	return []dbgen.GoldTripStop{{StopNo: 1, EngineOn: true}}, nil
}

func (q *fakeQ) ListTripGears(context.Context, uuid.UUID) ([]dbgen.GoldTripGear, error) {
	return []dbgen.GoldTripGear{{Gear: 2, RpmPerKmh: 40}}, nil
}

func (q *fakeQ) ListTripGearShifts(context.Context, uuid.UUID) ([]dbgen.GoldTripGearShift, error) {
	return []dbgen.GoldTripGearShift{{ShiftNo: 1, Direction: "subida", FromGear: 1, ToGear: 2}}, nil
}

func (q *fakeQ) LatestBootGeneration(context.Context, int32, int32) (int32, error) {
	return q.gen, q.genErr
}

func (q *fakeQ) ReadState(context.Context, int32) (dbgen.ReadStateRow, error) {
	return dbgen.ReadStateRow{HeadSeq: 7}, q.state
}

func ptr[T any](v T) *T { return &v }

func TestFinite(t *testing.T) {
	if finite(nil) != nil || finite(ptr(math.NaN())) != nil || finite(ptr(math.Inf(-1))) != nil {
		t.Fatal("ausentes e não finitos viram nil")
	}
	if v := finite(ptr(0.30000000000000004)); v == nil || *v != 0.30000000000000004 {
		t.Fatal("valor finito deve passar igual")
	}
}

func TestJSONHelpers(t *testing.T) {
	for _, in := range []string{"", "null", " null "} {
		if string(jsonList([]byte(in))) != "[]" || string(jsonObject([]byte(in))) != "{}" {
			t.Errorf("%q", in)
		}
	}
	if string(jsonList([]byte(`[1]`))) != "[1]" || string(jsonObject([]byte(`{"a":1}`))) != `{"a":1}` {
		t.Fatal("conteúdo deve ser repassado")
	}
	cases := map[string]bool{`{"t_ms": [], "lat": []}`: false, `{"t_ms": null}`: false, `{}`: false, `lixo`: false, ``: false,
		`{"t_ms": [1], "lat": [2], "lon": [3]}`: true}
	for in, keep := range cases {
		if got := routeOrNil([]byte(in)) != nil; got != keep {
			t.Errorf("%q: %v", in, got)
		}
	}
}

func TestSummaryFromSanitizes(t *testing.T) {
	nan := math.NaN()
	n := int64(10)
	s := summaryFrom(dbgen.GoldTripSummary{DistanceKm: ptr(3.5), DurationS: &nan, IdleRpm: ptr(math.Inf(1)), NReadings: &n,
		PidPeriodsMs: []byte(`{"0x0C":130,"0x0D":5}`), GpsCoveragePct: ptr(0.0)})
	if *s.DistanceKm != 3.5 || s.DurationS != nil || s.IdleRPM != nil || s.NReadings != 10 || s.PIDPeriodsMs["0x0C"] != 130 ||
		s.GPSCoveragePct == nil || *s.GPSCoveragePct != 0 {
		t.Fatalf("%+v", s)
	}
	if s := summaryFrom(dbgen.GoldTripSummary{PidPeriodsMs: []byte(`lixo`)}); s.NReadings != 0 || len(s.PIDPeriodsMs) != 0 {
		t.Fatalf("%+v", s)
	}
}

func TestInsertTranslatesDriverRepeated(t *testing.T) {
	q := &fakeQ{}
	r := &trips{q: q}
	got, err := r.Insert(context.Background(), trip.Draft{Category: "feira", DriverName: "João"})
	if err != nil || got.Status != trip.StatusPending || got.DriverName != "João" {
		t.Fatalf("%v %+v", err, got)
	}
	q.insert = &pgconn.PgError{Code: "23505", ConstraintName: "trip_driver_name_unique"}
	if _, err := r.Insert(context.Background(), trip.Draft{}); !errors.Is(err, trip.ErrDriverRepeated) {
		t.Fatalf("%v", err)
	}
	other := &pgconn.PgError{Code: "23505", ConstraintName: "outra"}
	q.insert = other
	if _, err := r.Insert(context.Background(), trip.Draft{}); !errors.Is(err, other) || errors.Is(err, trip.ErrDriverRepeated) {
		t.Fatalf("%v", err)
	}
	if err := r.NotifyTrip(context.Background(), uuid.MustParse("6f1c1f7e-3f0a-4a53-9d51-0c1d7a1b2c3d")); err != nil || q.notify != "6f1c1f7e-3f0a-4a53-9d51-0c1d7a1b2c3d" {
		t.Fatalf("%v %q", err, q.notify)
	}
}

func TestFindAndAnalysisNotFound(t *testing.T) {
	q := &fakeQ{tripErr: pgx.ErrNoRows}
	r := &trips{q: q}
	if _, err := r.Find(context.Background(), uuid.New()); !errors.Is(err, trip.ErrNotFound) {
		t.Fatalf("%v", err)
	}
	if _, err := r.Analysis(context.Background(), uuid.New()); !errors.Is(err, trip.ErrNotFound) {
		t.Fatalf("%v", err)
	}
	boom := errors.New("boom")
	q.tripErr = boom
	if _, err := r.Find(context.Background(), uuid.New()); !errors.Is(err, boom) {
		t.Fatalf("%v", err)
	}
	if _, err := r.Analysis(context.Background(), uuid.New()); !errors.Is(err, boom) {
		t.Fatalf("%v", err)
	}
}

func TestAnalysisNotReadyReadsOnlyTheTrip(t *testing.T) {
	q := &fakeQ{trip: dbgen.GoldTrip{Status: "waiting_data", DriverName: "João"}}
	a, err := (&trips{q: q}).Analysis(context.Background(), uuid.New())
	if err != nil || a.Trip.Status != trip.StatusWaitingData || a.Parts != nil || len(q.calls) != 1 {
		t.Fatalf("%v %+v %v", err, a, q.calls)
	}
}

func TestAnalysisReady(t *testing.T) {
	now := time.Now()
	q := &fakeQ{trip: dbgen.GoldTrip{Status: "ready", CalculatedAt: &now}, summary: dbgen.GoldTripSummary{Warnings: []byte(`null`), Parameters: []byte(`{"afr":13}`)},
		series: dbgen.GoldTripSeries{StepMs: 150, ExposureStepMs: 300, Series: []byte(`{"t_ms":[1]}`), Route: []byte(`{"t_ms":[],"lat":[]}`),
			RpmDistribution: []byte(`null`), PedalDistribution: []byte(`[{"de":0}]`)}}
	a, err := (&trips{q: q}).Analysis(context.Background(), uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	if string(a.Warnings) != "[]" || string(a.Params) != `{"afr":13}` || a.Series.Route != nil || string(a.Series.RPMDistribution) != "[]" ||
		string(a.Series.PedalDistribution) != `[{"de":0}]` || a.Series.StepMs != 150 {
		t.Fatalf("%+v", a)
	}
	if len(a.Parts) != 2 || a.Parts[0].Clock == nil || a.Parts[0].Clock.UncertaintyMs != 180 || a.Parts[1].Clock != nil {
		t.Fatalf("%+v", a.Parts)
	}
	if len(a.SpeedBands) != 1 || a.SpeedBands[0].ToKmh != nil {
		t.Fatalf("NaN deve virar nil: %+v", a.SpeedBands)
	}
	if len(a.Events) != 2 || a.Events[0].Peak != -6.8 || a.Events[0].Lat != nil || a.Events[1].Peak != 0 {
		t.Fatalf("%+v", a.Events)
	}
	if len(a.Stops) != 1 || !a.Stops[0].EngineOn || len(a.Gears) != 1 || a.Gears[0].Gear != 2 ||
		len(a.GearShifts) != 1 || a.GearShifts[0].To != 2 {
		t.Fatalf("%+v", a)
	}
}

func TestLatestGenerationAndStateRead(t *testing.T) {
	q := &fakeQ{gen: 3}
	if g, ok, err := (&boots{q}).LatestGeneration(context.Background(), 1, 8); err != nil || !ok || g != 3 {
		t.Fatalf("%d %v %v", g, ok, err)
	}
	q.genErr = pgx.ErrNoRows
	if _, ok, err := (&boots{q}).LatestGeneration(context.Background(), 1, 8); err != nil || ok {
		t.Fatalf("%v %v", ok, err)
	}
	boom := errors.New("boom")
	q.genErr = boom
	if _, _, err := (&boots{q}).LatestGeneration(context.Background(), 1, 8); !errors.Is(err, boom) {
		t.Fatalf("%v", err)
	}
	if st, err := (&states{q}).Read(context.Background(), 1); err != nil || st.HeadSeq != 7 {
		t.Fatalf("%v %+v", err, st)
	}
	q.state = pgx.ErrNoRows
	if _, err := (&states{q}).Read(context.Background(), 1); !errors.Is(err, dongle.ErrUnknown) {
		t.Fatalf("%v", err)
	}
	q.state = boom
	if _, err := (&states{q}).Read(context.Background(), 1); !errors.Is(err, boom) {
		t.Fatalf("%v", err)
	}
}
