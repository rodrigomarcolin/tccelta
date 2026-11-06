package trip

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"tccelta/backend/internal/adapter/memory"
	"tccelta/backend/internal/domain/dongle"
	"tccelta/backend/internal/domain/protocol"
	"tccelta/backend/internal/domain/timeline"
	domain "tccelta/backend/internal/domain/trip"
	"tccelta/backend/internal/domain/tripanalysis"
	"tccelta/backend/internal/port"
)

func i64(v int64) *int64 { return &v }

type env struct {
	db     *memory.Database
	create *CreateTrip
	get    *GetTrip
	an     *GetAnalysis
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db := memory.New()
	db.AddDongle(dongle.Dongle{ID: 1, DevID: protocol.DevID{1}, Active: true})
	db.SetState(1, dongle.State{HeadSeq: 120})
	db.AddDongle(dongle.Dongle{ID: 2, DevID: protocol.DevID{2}, Active: true}) // sem handshake: head_seq = 0
	for _, dongleID := range []int32{1, 2} {                                   // o boot 8 (geração 0) já chegou nos dois dongles
		db.BootRows = append(db.BootRows, &memory.Boot{PendingBoot: timeline.PendingBoot{BootKey: timeline.BootKey{DongleID: dongleID, BootID: 8}}})
	}
	return &env{db: db, create: NewCreateTrip(db), get: NewGetTrip(db.Repositories()), an: NewGetAnalysis(db.Repositories())}
}

func bootIn(name string) domain.Input {
	return domain.Input{DriverName: name, Category: "Feira", Fuel: "gasolina_e30", EntryType: "boot", DongleID: i64(1), BootID: i64(8)}
}

func TestCreateFreezesHeadSeqAndNotifies(t *testing.T) {
	e := newEnv(t)
	tr, err := e.create.Run(context.Background(), bootIn("João"))
	if err != nil {
		t.Fatal(err)
	}
	row := e.db.TripByID(tr.ID)
	if row == nil || row.Status != domain.StatusPending || row.TargetSeq != 120 || row.Category != "feira" || row.DriverName != "João" ||
		row.CalculatedAt != nil || row.BootGeneration == nil || *row.BootGeneration != 0 {
		t.Fatalf("%+v", row)
	}
	if len(e.db.TripNotified) != 1 || e.db.TripNotified[0] != tr.ID {
		t.Fatalf("notificações %v", e.db.TripNotified)
	}
}

func TestCreateDongleWithoutHandshakeIsAccepted(t *testing.T) {
	e := newEnv(t)
	in := bootIn("Ana")
	in.DongleID = i64(2)
	tr, err := e.create.Run(context.Background(), in)
	if err != nil || tr.TargetSeq != 0 {
		t.Fatalf("%v %+v", err, tr)
	}
}

func TestCreateUnknownDongle(t *testing.T) {
	e := newEnv(t)
	in := bootIn("Ana")
	in.DongleID = i64(99)
	if _, err := e.create.Run(context.Background(), in); !errors.Is(err, dongle.ErrUnknown) {
		t.Fatalf("%v", err)
	}
	if len(e.db.Trips) != 0 || len(e.db.TripNotified) != 0 {
		t.Fatal("nada deve ser gravado")
	}
}

func TestCreateInvalidInput(t *testing.T) {
	e := newEnv(t)
	in := bootIn("Ana")
	in.Fuel = "personalizado"
	if _, err := e.create.Run(context.Background(), in); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("%v", err)
	}
}

func TestCreateResolvesLatestGeneration(t *testing.T) {
	e := newEnv(t)
	for _, g := range []int32{0, 2, 1} {
		e.db.BootRows = append(e.db.BootRows, &memory.Boot{PendingBoot: timeline.PendingBoot{BootKey: timeline.BootKey{DongleID: 1, Generation: g, BootID: 8}}})
	}
	tr, err := e.create.Run(context.Background(), bootIn("João"))
	if err != nil || tr.BootGeneration == nil || *tr.BootGeneration != 2 {
		t.Fatalf("%v %+v", err, tr)
	}
	// geração informada vale, mesmo havendo outra maior
	in := bootIn("Maria")
	in.BootGeneration = i64(1)
	tr, err = e.create.Run(context.Background(), in)
	if err != nil || *tr.BootGeneration != 1 {
		t.Fatalf("%v %+v", err, tr)
	}
}

func TestCreateUnknownBoot(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	in := bootIn("Pedro")
	in.BootID = i64(77) // nunca chegou
	if _, err := e.create.Run(ctx, in); !errors.Is(err, domain.ErrBootNotFound) {
		t.Fatalf("%v", err)
	}
	// com geração informada também: o boot precisa existir
	in.BootGeneration = i64(0)
	if _, err := e.create.Run(ctx, in); !errors.Is(err, domain.ErrBootNotFound) {
		t.Fatalf("%v", err)
	}
	// o boot 8 existe no dongle 1 e no 2, mas não num dongle sem ele
	e.db.AddDongle(dongle.Dongle{ID: 3, DevID: protocol.DevID{3}, Active: true})
	other := bootIn("Rui")
	other.DongleID = i64(3)
	if _, err := e.create.Run(ctx, other); !errors.Is(err, domain.ErrBootNotFound) {
		t.Fatalf("%v", err)
	}
	if len(e.db.Trips) != 0 || len(e.db.TripNotified) != 0 {
		t.Fatal("o boot desconhecido não deixa viagem nem NOTIFY")
	}
}

func TestCreateInterval(t *testing.T) {
	e := newEnv(t)
	t0, t1 := "2026-10-28T08:00:00-03:00", "2026-10-28T09:00:00-03:00"
	tr, err := e.create.Run(context.Background(), domain.Input{DriverName: "Lia", Category: "x", Fuel: "diesel", EntryType: "intervalo",
		DongleID: i64(1), T0: &t0, T1: &t1})
	if err != nil || tr.EntryType != domain.EntryTypeInterval || tr.IntervalStart == nil || tr.BootID != nil {
		t.Fatalf("%v %+v", err, tr)
	}
}

func TestCreateRepeatedDriver(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	first, err := e.create.Run(ctx, bootIn("  JOÃO "))
	if err != nil {
		t.Fatal(err)
	}
	notified := len(e.db.TripNotified)
	if _, err := e.create.Run(ctx, bootIn("joão")); !errors.Is(err, domain.ErrDriverRepeated) {
		t.Fatalf("%v", err)
	}
	if len(e.db.TripNotified) != notified || len(e.db.Trips) != 1 {
		t.Fatal("a tentativa repetida não deixa rastro")
	}
	// outra categoria aceita o mesmo nome
	other := bootIn("joão")
	other.Category = "outra"
	if _, err := e.create.Run(ctx, other); err != nil {
		t.Fatal(err)
	}
	// com a primeira em failed, o nome é liberado
	e.db.TripByID(first.ID).Status = domain.StatusFailed
	if _, err := e.create.Run(ctx, bootIn("João")); err != nil {
		t.Fatal(err)
	}
}

func TestGetTrip(t *testing.T) {
	e := newEnv(t)
	tr, _ := e.create.Run(context.Background(), bootIn("João"))
	got, err := e.get.Run(context.Background(), tr.ID)
	if err != nil || got.ID != tr.ID {
		t.Fatalf("%v %+v", err, got)
	}
	if _, err := e.get.Run(context.Background(), uuid.New()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("%v", err)
	}
}

func TestGetAnalysis(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if _, err := e.an.Run(ctx, uuid.New()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("%v", err)
	}
	tr, _ := e.create.Run(ctx, bootIn("João"))
	for _, st := range []string{domain.StatusPending, domain.StatusWaitingData, domain.StatusFailed} {
		e.db.TripByID(tr.ID).Status = st
		_, err := e.an.Run(ctx, tr.ID)
		var nr domain.NotReadyError
		if !errors.Is(err, domain.ErrNotReady) || !errors.As(err, &nr) || nr.Status != st {
			t.Fatalf("%s: %v", st, err)
		}
	}
	// pronta: devolve o que o Save gravou
	dist := 3.5
	res := tripanalysis.Result{Summary: tripanalysis.Summary{DistanceKm: &dist, NReadings: 10}, Params: tripanalysis.DefaultParams(),
		Series: tripanalysis.SeriesData{StepMs: 150, ExposureStepMs: 300}}
	e.db.TripByID(tr.ID).Status = domain.StatusPending
	rp := e.db.Repositories()
	if err := rp.Trips.Save(ctx, tr.ID, domain.Calculation{Result: res, CalculatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	a, err := e.an.Run(ctx, tr.ID)
	if err != nil || *a.Summary.DistanceKm != 3.5 || a.Series.StepMs != 150 || string(a.Warnings) != "[]" || a.Series.Route != nil {
		t.Fatalf("%v %+v", err, a)
	}
	res.Series.Route.TMs = []int64{1}
	_ = rp.Trips.Save(ctx, tr.ID, domain.Calculation{Result: res, CalculatedAt: time.Now()})
	if a, _ := e.an.Run(ctx, tr.ID); a.Series.Route == nil {
		t.Fatal("rota com pontos deve ser repassada")
	}
}

// failingTrips faz a leitura da análise falhar: o erro do banco sobe sem ser reinterpretado.
type failingTrips struct{ port.Trips }

func (failingTrips) Analysis(context.Context, uuid.UUID) (domain.Analysis, error) {
	return domain.Analysis{}, port.ErrDatabaseBusy
}

func TestGetAnalysisPropagatesErrors(t *testing.T) {
	r := memory.New().Repositories()
	r.Trips = failingTrips{r.Trips}
	if _, err := NewGetAnalysis(r).Run(context.Background(), uuid.New()); !errors.Is(err, port.ErrDatabaseBusy) {
		t.Fatalf("%v", err)
	}
}
