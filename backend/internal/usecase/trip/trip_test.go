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
	for _, dongleID := range []int32{1, 2} {                                   // os boots 8, 9 e 10 (geração 0) já chegaram nos dois dongles
		for _, bootID := range []int32{8, 9, 10} {
			db.BootRows = append(db.BootRows, &memory.Boot{PendingBoot: timeline.PendingBoot{BootKey: timeline.BootKey{DongleID: dongleID, BootID: bootID}}})
		}
	}
	return &env{db: db, create: NewCreateTrip(db), get: NewGetTrip(db.Repositories()), an: NewGetAnalysis(db.Repositories())}
}

func bootIn(name string) domain.Input {
	return domain.Input{DriverName: name, Category: "Feira", Fuel: "gasolina_e30", EntryType: "boot", DongleID: i64(1), BootID: i64(8)}
}

func bootInID(name string, bootID int64) domain.Input {
	in := bootIn(name)
	in.BootID = i64(bootID)
	return in
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
	// o mesmo nome em outro boot (a mesma viagem seria viagem_repetida)
	if _, err := e.create.Run(ctx, bootInID("joão", 9)); !errors.Is(err, domain.ErrDriverRepeated) {
		t.Fatalf("%v", err)
	}
	if len(e.db.TripNotified) != notified || len(e.db.Trips) != 1 {
		t.Fatal("a tentativa repetida não deixa rastro")
	}
	// outra categoria aceita o mesmo nome
	other := bootInID("joão", 9)
	other.Category = "outra"
	if _, err := e.create.Run(ctx, other); err != nil {
		t.Fatal(err)
	}
	// com a primeira em failed, o nome é liberado
	e.db.TripByID(first.ID).Status = domain.StatusFailed
	if _, err := e.create.Run(ctx, bootInID("João", 10)); err != nil {
		t.Fatal(err)
	}
}

func TestCreateRepeatedTrip(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	first, err := e.create.Run(ctx, bootIn("João"))
	if err != nil {
		t.Fatal(err)
	}
	notified := len(e.db.TripNotified)
	// mesmo boot: recusa para o mesmo motorista, para outro e para outra categoria; a viagem repetida vence o nome repetido
	for _, in := range []domain.Input{bootIn("João"), bootIn("Maria"), func() domain.Input { i := bootIn("Maria"); i.Category = "outra"; return i }()} {
		_, err := e.create.Run(ctx, in)
		var rep domain.RepeatedTripError
		if !errors.Is(err, domain.ErrTripRepeated) || errors.Is(err, domain.ErrDriverRepeated) || !errors.As(err, &rep) ||
			rep.ID != first.ID || rep.DriverName != "João" {
			t.Fatalf("%v", err)
		}
	}
	if len(e.db.TripNotified) != notified || len(e.db.Trips) != 1 {
		t.Fatal("a tentativa repetida não deixa rastro")
	}
	// a geração informada igual à resolvida (0) é a mesma viagem; outra geração é outro boot
	in := bootIn("Maria")
	in.BootGeneration = i64(0)
	if _, err := e.create.Run(ctx, in); !errors.Is(err, domain.ErrTripRepeated) {
		t.Fatalf("%v", err)
	}
	e.db.BootRows = append(e.db.BootRows, &memory.Boot{PendingBoot: timeline.PendingBoot{BootKey: timeline.BootKey{DongleID: 1, Generation: 1, BootID: 8}}})
	in = bootIn("Maria")
	in.BootGeneration = i64(1)
	if _, err := e.create.Run(ctx, in); err != nil {
		t.Fatalf("outra geração do boot não é a mesma viagem: %v", err)
	}
	// outro dongle com o mesmo boot_id também é outra viagem
	other := bootIn("Rui")
	other.DongleID = i64(2)
	if _, err := e.create.Run(ctx, other); err != nil {
		t.Fatal(err)
	}
	// viagem failed ou excluída não bloqueia: a failed libera
	e.db.TripByID(first.ID).Status = domain.StatusFailed
	ana := bootIn("Ana")
	ana.BootGeneration = i64(0)
	if _, err := e.create.Run(ctx, ana); err != nil {
		t.Fatalf("failed não bloqueia: %v", err)
	}
}

func TestCreateRepeatedInterval(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	t0, t1, t2 := "2026-10-28T08:00:00-03:00", "2026-10-28T09:00:00-03:00", "2026-10-28T09:30:00-03:00"
	mk := func(name, from, to string) domain.Input {
		return domain.Input{DriverName: name, Category: "x", Fuel: "diesel", EntryType: "intervalo", DongleID: i64(1), T0: &from, T1: &to}
	}
	first, err := e.create.Run(ctx, mk("Lia", t0, t1))
	if err != nil {
		t.Fatal(err)
	}
	// o mesmo instante em outro fuso é o mesmo intervalo
	utc0, utc1 := "2026-10-28T11:00:00Z", "2026-10-28T12:00:00Z"
	var rep domain.RepeatedTripError
	if _, err := e.create.Run(ctx, mk("Beto", utc0, utc1)); !errors.As(err, &rep) || rep.ID != first.ID || rep.DriverName != "Lia" {
		t.Fatalf("%v", err)
	}
	// só t0 e t1 exatamente iguais colidem: um intervalo sobreposto passa (limite conhecido)
	if _, err := e.create.Run(ctx, mk("Beto", t0, t2)); err != nil {
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

// raceTx simula a corrida do Insert: o primeiro FindSame não vê a viagem que o Insert, em seguida, recusa pelo índice único.
type raceTx struct {
	db          *memory.Database
	hidden      int // FindSame que ainda não enxergam nada
	failInserts int // Insert que ainda são recusados pelo índice único
	retries     int
}

type raceTrips struct {
	port.Trips
	tx *raceTx
}

func (r raceTrips) FindSame(ctx context.Context, d domain.Draft) (domain.Trip, bool, error) {
	if r.tx.hidden > 0 {
		r.tx.hidden--
		return domain.Trip{}, false, nil
	}
	return r.Trips.FindSame(ctx, d)
}

func (x *raceTx) InTransaction(ctx context.Context, fn func(ctx context.Context, r port.Repositories) error) error {
	x.retries++
	return x.db.InTransaction(ctx, func(ctx context.Context, r port.Repositories) error {
		r.Trips = raceTrips{r.Trips, x}
		return fn(ctx, r)
	})
}

func TestCreateRaceReadsWinnerInAnotherTransaction(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	first, err := e.create.Run(ctx, bootIn("João"))
	if err != nil {
		t.Fatal(err)
	}
	tx := &raceTx{db: e.db, hidden: 1}
	_, err = NewCreateTrip(tx).Run(ctx, bootIn("Maria"))
	var rep domain.RepeatedTripError
	if !errors.As(err, &rep) || !errors.Is(err, domain.ErrTripRepeated) || rep.ID != first.ID || rep.DriverName != "João" {
		t.Fatalf("%v", err)
	}
	if tx.retries != 2 { // a tentativa que abortou e a que lê o vencedor
		t.Fatalf("transações: %d", tx.retries)
	}
	if len(e.db.Trips) != 1 {
		t.Fatal("a corrida perdida não deixa viagem")
	}
}

func TestCreateRaceWinnerGoneRetries(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	first, _ := e.create.Run(ctx, bootIn("João"))
	// o vencedor falhou entre o Insert recusado e a releitura: a segunda tentativa cria a viagem
	tx := &raceTx{db: e.db, failInserts: 1}
	e.db.TripByID(first.ID).Status = domain.StatusFailed
	tr, err := NewCreateTrip(tx).Run(ctx, bootIn("Maria"))
	if err != nil || tr.DriverName != "Maria" {
		t.Fatalf("%v %+v", err, tr)
	}
}

func TestListTrips(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	base := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	for i, name := range []string{"Ana", "Bruno", "Ana Lúcia", "Caio", "100%_Teste"} {
		row := e.db.AddTrip(domain.Trip{Category: "feira", DriverName: name, Fuel: "diesel", DongleID: 1, EntryType: domain.EntryTypeBoot,
			CreatedAt: base.Add(time.Duration(i) * time.Minute)})
		_ = row
	}
	l := NewListTrips(e.db.Repositories())
	q, _ := domain.NewListQuery(i64(2), "", "")
	p, err := l.Run(ctx, q)
	if err != nil || p.Total != 5 || len(p.Items) != 2 || p.Next == nil || p.Items[0].DriverName != "100%_Teste" || p.Items[1].DriverName != "Caio" {
		t.Fatalf("%v %+v", err, p)
	}
	// o cursor continua de onde parou, sem repetir, e a última página não tem próxima
	var seen []string
	for {
		for _, it := range p.Items {
			seen = append(seen, it.DriverName)
		}
		if p.Next == nil {
			break
		}
		q.Cursor = p.Next
		if p, err = l.Run(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	if len(seen) != 5 || seen[2] != "Ana Lúcia" || seen[4] != "Ana" {
		t.Fatalf("%v", seen)
	}
	// busca por trecho, sem diferenciar caixa; o total respeita a busca
	q, _ = domain.NewListQuery(nil, "", " ANA ")
	if p, _ = l.Run(ctx, q); p.Total != 2 || len(p.Items) != 2 || p.Next != nil {
		t.Fatalf("%+v", p)
	}
	// sem resultado: lista vazia, nunca nula
	q, _ = domain.NewListQuery(nil, "", "zzz")
	if p, _ = l.Run(ctx, q); p.Total != 0 || p.Items == nil || len(p.Items) != 0 {
		t.Fatalf("%+v", p)
	}
}

func TestListTripsFields(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	rp := e.db.Repositories()
	tr, _ := e.create.Run(ctx, bootIn("João"))
	l := NewListTrips(rp)
	q, _ := domain.NewListQuery(nil, "", "")
	if p, _ := l.Run(ctx, q); p.Items[0].Route != nil || p.Items[0].DistanceKm != nil {
		t.Fatalf("antes de ficar pronta não há resumo: %+v", p.Items[0])
	}
	dist, dur := 3.5, 600.0
	start, end := time.Date(2026, 10, 28, 8, 0, 0, 0, time.UTC), time.Date(2026, 10, 28, 8, 10, 0, 0, time.UTC)
	res := tripanalysis.Result{Summary: tripanalysis.Summary{DistanceKm: &dist, DurationS: &dur, StartedAt: &start, EndedAt: &end},
		Params: tripanalysis.DefaultParams(), Series: tripanalysis.SeriesData{StepMs: 150, ExposureStepMs: 300}}
	if err := rp.Trips.Save(ctx, tr.ID, domain.Calculation{Result: res, CalculatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	p, _ := l.Run(ctx, q)
	it := p.Items[0]
	if s, en := it.Period(); *it.DistanceKm != 3.5 || *it.DurationS != 600 || !s.Equal(start) || !en.Equal(end) || it.Route != nil {
		t.Fatalf("pronta sem GPS: %+v", it)
	}
	res.Series.Route = tripanalysis.Route{TMs: []int64{0, 1000}, Lat: []float64{-23.5, -23.6}, Lon: []float64{-46.6, -46.7},
		Segments: tripanalysis.RouteSegments{Band: []string{"v1"}}}
	_ = rp.Trips.Save(ctx, tr.ID, domain.Calculation{Result: res, CalculatedAt: time.Now()})
	if p, _ = l.Run(ctx, q); len(p.Items[0].Route) != 1 || len(p.Items[0].Route[0]) != 2 {
		t.Fatalf("%+v", p.Items[0].Route)
	}
}

func (r raceTrips) Insert(ctx context.Context, d domain.Draft) (domain.Trip, error) {
	if r.tx.failInserts > 0 {
		r.tx.failInserts--
		return domain.Trip{}, domain.ErrTripRepeated
	}
	return r.Trips.Insert(ctx, d)
}
