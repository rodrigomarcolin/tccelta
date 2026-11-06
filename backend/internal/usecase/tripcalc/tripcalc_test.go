package tripcalc

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"tccelta/backend/internal/adapter/memory"
	"tccelta/backend/internal/domain/dongle"
	"tccelta/backend/internal/domain/protocol"
	"tccelta/backend/internal/domain/timeline"
	"tccelta/backend/internal/domain/trip"
	"tccelta/backend/internal/domain/tripanalysis"
	"tccelta/backend/internal/port"
	"tccelta/backend/internal/port/porttest"
)

const dongleID = 7

var base = time.Unix(1_800_000_000, 0).UTC()

func i32(v int32) *int32 { return &v }
func i64(v int64) *int64 { return &v }

type metrics struct {
	porttest.Nop
	trips       map[string]int
	partial     int
	provisional map[string]int
	queue       [2]int64
}

func (m *metrics) Trip(r string)            { m.trips[r]++ }
func (m *metrics) TripPartial()             { m.partial++ }
func (m *metrics) TripProvisional(a string) { m.provisional[a]++ }
func (m *metrics) TripQueue(w, r int64)     { m.queue = [2]int64{w, r} }

type env struct {
	db    *memory.Database
	calc  *Calculator
	met   *metrics
	clock *porttest.AdjustableClock
}

func newEnvWith(t *testing.T, wrap func(*memory.Database) (port.Repositories, port.Transactor)) *env {
	t.Helper()
	db := memory.New()
	db.AddDongle(dongle.Dongle{ID: dongleID, DevID: protocol.DevID{7}, Profile: protocol.ProfileHMAC, Active: true})
	m := &metrics{trips: map[string]int{}, provisional: map[string]int{}}
	clock := &porttest.AdjustableClock{T: base}
	repos, tx := db.Repositories(), port.Transactor(db)
	if wrap != nil {
		repos, tx = wrap(db)
	}
	cfg := Config{WaitNoBatch: 10 * time.Second, FinalLimit: 20 * time.Second, Timeline: timeline.DefaultConfig()}
	return &env{db: db, met: m, clock: clock,
		calc: New(repos, tx, db, m, clock.Now, slog.New(slog.NewTextHandler(io.Discard, nil)), cfg)}
}

func newEnv(t *testing.T) *env { return newEnvWith(t, nil) }

// boot cadastra um boot com a faixa de seq e, se withTime, a reta hora(t) = base + t.
func (e *env) boot(id int32, firstSeq, lastSeq int64, withTime bool) *memory.Boot {
	b := &memory.Boot{LastSeq: i64(lastSeq)}
	b.BootKey = timeline.BootKey{DongleID: dongleID, Generation: 0, BootID: id}
	b.FirstSeq, b.FirstTMs, b.LastTMs, b.MaxOffsetMs = i64(firstSeq), i64(0), i64(59000), func() *float64 { v := 2e12; return &v }()
	if withTime {
		e.convert(b, float64(base.UnixMilli()))
	}
	e.db.BootRows = append(e.db.BootRows, b)
	return b
}

// convert dá ao boot a reta hora(t) = a + t, com um grupo de âncoras de meia largura 50 ms.
func (e *env) convert(b *memory.Boot, a float64) {
	src, zero, unc := "anchors", 0.0, int32(100)
	b.AMs, b.B, b.TimeSource, b.BaseUncertaintyMs = &a, &zero, &src, &unc
	e.db.Groups[b.BootKey] = []timeline.Group{{Lo: a - 50, Hi: a + 50, TCenter: 30000, FirstT: 0, LastT: 59000, N: 2}}
}

// data grava n segundos de leituras (rpm 1500 e 50 km/h a cada segundo) e um bloco por seq do boot.
func (e *env) data(b *memory.Boot, n int, withGPS bool) {
	recv := base.Add(-time.Minute)
	for s := *b.FirstSeq; s <= *b.LastSeq; s++ {
		e.db.SourceBlocks = append(e.db.SourceBlocks, memory.SourceBlock{DongleID: dongleID, Seq: s, Generation: b.Generation,
			BootID: b.BootID, T0Ms: (s - *b.FirstSeq) * 10000, ReceivedAt: recv})
	}
	for i := 0; i < n; i++ {
		tm := int64(i) * 1000
		for _, r := range []struct {
			pid int16
			v   float64
		}{{0x0C, 1500}, {0x0D, 50}} {
			e.db.SourceReadings = append(e.db.SourceReadings, memory.SourceReading{DongleID: dongleID, Generation: b.Generation,
				BootID: b.BootID, PID: r.pid, TMs: tm, Value: r.v, ReceivedAt: recv})
		}
		if withGPS {
			e.db.SourcePositions = append(e.db.SourcePositions, memory.SourcePosition{DongleID: dongleID, Generation: b.Generation,
				BootID: b.BootID, TMs: tm, Lat: -25 - float64(i)*1e-4, Lon: -49, ReceivedAt: recv})
		}
	}
}

func (e *env) progress(contiguous int64, lastAck time.Time) {
	e.db.SetState(dongleID, dongle.State{ContiguousSeq: contiguous, MaxSeenSeq: contiguous, HeadSeq: contiguous})
	if !lastAck.IsZero() {
		e.db.SetLastAck(dongleID, lastAck)
	}
}

func (e *env) bootTrip(boot int32, target int64) *memory.TripRow {
	return e.db.AddTrip(trip.Trip{DongleID: dongleID, Category: "feira", DriverName: "A", Fuel: "gasolina_e30", EntryType: "boot",
		BootGeneration: i32(0), BootID: i32(boot), TargetSeq: target, CreatedAt: e.clock.Now()})
}

func (e *env) intervalTrip(from, to time.Time, target int64) *memory.TripRow {
	return e.db.AddTrip(trip.Trip{DongleID: dongleID, Category: "feira", DriverName: "B", Fuel: "gasolina_e30", EntryType: "interval",
		IntervalStart: &from, IntervalEnd: &to, TargetSeq: target, CreatedAt: e.clock.Now()})
}

func (e *env) evaluate(t *testing.T) []Ready {
	t.Helper()
	r, err := e.calc.Evaluate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func (e *env) calculate(t *testing.T, r Ready) {
	t.Helper()
	found, err := e.calc.Calculate(context.Background(), r.ID, r.ByLimit)
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
}

func hasWarning(r tripanalysis.Result, code string) bool {
	for _, w := range r.Warnings {
		if w.Code == code {
			return true
		}
	}
	return false
}

// Regra de espera (critério 8): silêncio de 10 s com contiguous_seq no alvo; os estados passam por pending, waiting_data e ready.
func TestWaitsForSilence(t *testing.T) {
	e := newEnv(t)
	b := e.boot(1, 1, 5, true)
	e.data(b, 30, false)
	tr := e.bootTrip(1, 5)
	e.progress(5, base)
	if tr.Status != trip.StatusPending {
		t.Fatal(tr.Status)
	}
	if r := e.evaluate(t); len(r) != 0 || tr.Status != trip.StatusWaitingData {
		t.Fatalf("pronta cedo demais: %v %s", r, tr.Status)
	}
	e.clock.Advance(9 * time.Second)
	if r := e.evaluate(t); len(r) != 0 {
		t.Fatalf("9 s de silêncio: %v", r)
	}
	e.clock.Advance(time.Second)
	r := e.evaluate(t)
	if len(r) != 1 || r[0].ByLimit {
		t.Fatalf("10 s de silêncio: %+v", r)
	}
	e.calculate(t, r[0])
	if tr.Status != trip.StatusReady || tr.Calc == nil || tr.Saves != 1 || tr.CalculatedAt == nil || *tr.CalculationVersion != "1" {
		t.Fatalf("%+v", tr.Trip)
	}
	if hasWarning(tr.Calc.Result, tripanalysis.WarnPartialData) || e.met.partial != 0 || e.met.trips["ready"] != 1 {
		t.Fatalf("avisos %+v métricas %+v", tr.Calc.Result.Warnings, e.met.trips)
	}
	if len(e.evaluate(t)) != 0 {
		t.Fatal("viagem pronta não volta para a avaliação")
	}
}

// Dongle desligado (sem nenhum lote confirmado): calcula na hora.
func TestNoAckIsReady(t *testing.T) {
	e := newEnv(t)
	e.bootTrip(1, 0)
	e.progress(0, time.Time{})
	if r := e.evaluate(t); len(r) != 1 {
		t.Fatalf("%v", r)
	}
}

// Critério 8: lotes contínuos com contiguous_seq abaixo do alvo: aos 20 s, com dados_parciais.
func TestFinalLimitIsPartial(t *testing.T) {
	e := newEnv(t)
	b := e.boot(1, 1, 5, true)
	e.data(b, 30, false)
	tr := e.bootTrip(1, 9)
	e.progress(5, base)
	for i := 0; i < 19; i++ {
		e.clock.Advance(time.Second)
		e.progress(5, e.clock.Now()) // lotes chegando: nunca há 10 s de silêncio
		if r := e.evaluate(t); len(r) != 0 {
			t.Fatalf("aos %d s: %v", i+1, r)
		}
	}
	e.clock.Advance(time.Second)
	e.progress(5, e.clock.Now())
	r := e.evaluate(t)
	if len(r) != 1 || !r[0].ByLimit {
		t.Fatalf("aos 20 s: %+v", r)
	}
	e.calculate(t, r[0])
	if !hasWarning(tr.Calc.Result, tripanalysis.WarnPartialData) || e.met.partial != 1 || !tr.Provisional {
		t.Fatalf("%+v provisória=%v", tr.Calc.Result.Warnings, tr.Provisional)
	}
	if *tr.CalculatedThroughSeq != 5 {
		t.Fatalf("through %d", *tr.CalculatedThroughSeq)
	}
}

// Critério 8: intervalo com bloco de hora >= t1 é calculado sem esperar os 10 s.
func TestIntervalReadyByBlockTime(t *testing.T) {
	e := newEnv(t)
	b := e.boot(1, 1, 5, true) // blocos com t0 = 0, 10, 20, 30, 40 s
	e.data(b, 50, false)
	e.progress(5, base)
	early := e.intervalTrip(base.Add(10*time.Second), base.Add(35*time.Second), 5)
	late := e.intervalTrip(base.Add(10*time.Second), base.Add(55*time.Second), 5) // nenhum bloco chegou a t1
	r := e.evaluate(t)
	if len(r) != 1 || r[0].ID != early.ID || late.Status != trip.StatusWaitingData {
		t.Fatalf("%+v", r)
	}
}

func TestIntervalWithoutConversionWaitsForSilence(t *testing.T) {
	e := newEnv(t)
	b := e.boot(1, 1, 5, false)
	e.data(b, 50, false)
	e.progress(5, base)
	e.intervalTrip(base.Add(10*time.Second), base.Add(35*time.Second), 5)
	if r := e.evaluate(t); len(r) != 0 {
		t.Fatalf("%+v", r)
	}
	e.clock.Advance(10 * time.Second)
	if r := e.evaluate(t); len(r) != 1 {
		t.Fatalf("%+v", r)
	}
}

// Prova de completude (critério 10): sem boot seguinte a viagem é provisória; com ele, não.
func TestProvisionalDependsOnNextBoot(t *testing.T) {
	e := newEnv(t)
	b := e.boot(1, 1, 5, true)
	e.data(b, 30, true)
	tr := e.bootTrip(1, 5)
	e.progress(5, base)
	e.clock.Advance(10 * time.Second)
	e.calculate(t, e.evaluate(t)[0])
	if !tr.Provisional {
		t.Fatal("sem boot seguinte a viagem é provisória")
	}
	if tr.Calc.Result.Summary.DistanceKm == nil || len(tr.Calc.Result.Series.Route.TMs) != 30 {
		t.Fatalf("%+v", tr.Calc.Result.Summary)
	}
	if *tr.FirstSeq != 1 || *tr.LastSeq != 5 {
		t.Fatalf("faixa %d-%d", *tr.FirstSeq, *tr.LastSeq)
	}
	// um boot seguinte começou, mas ainda além do contiguous_seq: continua provisória
	next := e.boot(2, 8, 9, true)
	e.progress(5, base)
	if _, err := e.calc.CheckProvisional(context.Background()); err != nil || !tr.Provisional || tr.RecalcPending {
		t.Fatalf("err=%v provisória=%v recalc=%v", err, tr.Provisional, tr.RecalcPending)
	}
	// agora o contiguous_seq passa do início do boot seguinte: prova de completude, sem recalcular
	e.progress(8, base)
	saves := tr.Saves
	n, err := e.calc.CheckProvisional(context.Background())
	if err != nil || n != 0 || tr.Provisional || tr.RecalcPending || tr.Saves != saves || e.met.provisional["completed"] != 1 {
		t.Fatalf("n=%d err=%v provisória=%v recalc=%v saves=%d", n, err, tr.Provisional, tr.RecalcPending, tr.Saves)
	}
	_ = next
}

// Critério 10: bloco novo dentro da viagem provisória gera recalc_pending e o recálculo atualiza calculated_through_seq.
func TestLateDataTriggersRecalc(t *testing.T) {
	e := newEnv(t)
	b := e.boot(1, 1, 5, true)
	e.data(b, 30, false)
	tr := e.bootTrip(1, 5)
	e.progress(5, base)
	e.clock.Advance(10 * time.Second)
	e.calculate(t, e.evaluate(t)[0])
	if *tr.CalculatedThroughSeq != 5 {
		t.Fatal(*tr.CalculatedThroughSeq)
	}
	before := *tr.Calc.Result.Summary.DistanceKm
	// chegam mais blocos do mesmo boot
	*b.LastSeq = 7
	e.db.SourceBlocks = append(e.db.SourceBlocks, memory.SourceBlock{DongleID: dongleID, Seq: 6, BootID: 1, T0Ms: 50000, ReceivedAt: base.Add(-time.Minute)},
		memory.SourceBlock{DongleID: dongleID, Seq: 7, BootID: 1, T0Ms: 60000, ReceivedAt: base.Add(-time.Minute)})
	for i := 30; i < 45; i++ {
		for _, r := range []struct {
			pid int16
			v   float64
		}{{0x0C, 1500}, {0x0D, 50}} {
			e.db.SourceReadings = append(e.db.SourceReadings, memory.SourceReading{DongleID: dongleID, BootID: 1, PID: r.pid,
				TMs: int64(i) * 1000, Value: r.v, ReceivedAt: base.Add(-time.Minute)})
		}
	}
	e.progress(7, e.clock.Now())
	n, err := e.calc.CheckProvisional(context.Background())
	if err != nil || n != 1 || !tr.RecalcPending || e.db.TripNotifications != 1 || e.met.provisional["recalc_marked"] != 1 {
		t.Fatalf("n=%d err=%v recalc=%v notif=%d", n, err, tr.RecalcPending, e.db.TripNotifications)
	}
	found, err := e.calc.Recalc(context.Background())
	if err != nil || !found || tr.RecalcPending || *tr.CalculatedThroughSeq != 7 || tr.Saves != 2 || e.met.trips["recalculated"] != 1 {
		t.Fatalf("found=%v err=%v %+v", found, err, tr.Trip)
	}
	if after := *tr.Calc.Result.Summary.DistanceKm; after <= before {
		t.Fatalf("distância %v -> %v", before, after)
	}
	if found, _ := e.calc.Recalc(context.Background()); found {
		t.Fatal("a fila de recálculo deveria estar vazia")
	}
}

// Viagem completa não é recalculada por âncora nova nem por dados de outro boot.
func TestCompleteTripNotRecalculated(t *testing.T) {
	e := newEnv(t)
	b := e.boot(1, 1, 5, true)
	e.data(b, 30, false)
	e.boot(2, 6, 9, true)
	tr := e.bootTrip(1, 5)
	e.progress(9, base)
	e.clock.Advance(10 * time.Second)
	e.calculate(t, e.evaluate(t)[0])
	if tr.Provisional {
		t.Fatal("o boot seguinte já começou")
	}
	e.convert(b, float64(base.UnixMilli())+500) // âncora nova
	if n, err := e.calc.CheckProvisional(context.Background()); err != nil || n != 0 || tr.RecalcPending {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

// Critério 12: o boot sem hora ganha conversão e o ticker marca o recálculo, que o inclui.
func TestBootWithoutTimeGainsConversion(t *testing.T) {
	e := newEnv(t)
	good := e.boot(1, 1, 5, true)
	e.data(good, 60, false)
	late := e.boot(2, 6, 9, false)
	e.data(late, 60, false)
	tr := e.intervalTrip(base.Add(20*time.Second), base.Add(100*time.Second), 9)
	e.progress(9, base)
	e.clock.Advance(10 * time.Second)
	e.calculate(t, e.evaluate(t)[0])
	res := tr.Calc.Result
	if !hasWarning(res, tripanalysis.WarnBootNoTime) || len(res.Parts) != 1 {
		t.Fatalf("avisos %+v partes %d", res.Warnings, len(res.Parts))
	}
	for _, w := range res.Warnings {
		if w.Code == tripanalysis.WarnBootNoTime && (w.Detail["boot_id"] != int32(2) || w.Detail["boot_generation"] != int32(0)) {
			t.Fatalf("detalhe %+v", w.Detail)
		}
	}
	if !tr.Provisional {
		t.Fatal("sem prova de completude a viagem é provisória")
	}
	if n, _ := e.calc.CheckProvisional(context.Background()); n != 0 || tr.RecalcPending {
		t.Fatalf("o boot ainda não tem hora: n=%d", n)
	}
	e.convert(late, float64(base.UnixMilli())+60000) // o boot 2 começa 60 s depois do 1
	n, err := e.calc.CheckProvisional(context.Background())
	if err != nil || n != 1 || !tr.RecalcPending {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if found, err := e.calc.Recalc(context.Background()); err != nil || !found {
		t.Fatalf("%v %v", found, err)
	}
	res = tr.Calc.Result
	if hasWarning(res, tripanalysis.WarnBootNoTime) || len(res.Parts) != 2 || res.Parts[1].BootID != 2 {
		t.Fatalf("avisos %+v partes %+v", res.Warnings, res.Parts)
	}
}

// Critério 15: intervalo dentro de um boot: uma parte, t_ms contado desde t0, só com os dados de dentro.
func TestIntervalInsideOneBoot(t *testing.T) {
	e := newEnv(t)
	b := e.boot(1, 1, 5, true)
	e.data(b, 60, true)
	tr := e.intervalTrip(base.Add(10*time.Second), base.Add(40*time.Second), 5)
	e.progress(5, base)
	e.clock.Advance(10 * time.Second)
	e.calculate(t, e.evaluate(t)[0])
	res := tr.Calc.Result
	if len(res.Parts) != 1 || res.Parts[0].DataStartTMs != 0 || res.Parts[0].DataEndTMs != 29000 {
		t.Fatalf("partes %+v", res.Parts)
	}
	s := res.Summary
	if *s.StartTMs != 0 || *s.EndTMs != 29000 || *s.ClockOffsetMs != base.Add(10*time.Second).UnixMilli() || *s.ClockUncertaintyMs != 51 {
		t.Fatalf("resumo %+v", s)
	}
	if !s.StartedAt.Equal(base.Add(10 * time.Second)) {
		t.Fatalf("início %v", s.StartedAt)
	}
	if len(res.Series.Route.TMs) != 30 || res.Series.Route.TMs[0] != 0 {
		t.Fatalf("rota %v", res.Series.Route.TMs)
	}
	if *tr.FirstSeq != 1 || *tr.LastSeq != 5 {
		t.Fatalf("faixa %d-%d", *tr.FirstSeq, *tr.LastSeq)
	}
}

// Critério 16: dois boots, duas partes, sem_dados_s igual ao intervalo entre elas; incerteza maior que a sobreposição.
func TestIntervalTwoBoots(t *testing.T) {
	e := newEnv(t)
	b1 := e.boot(1, 1, 5, true)
	e.data(b1, 60, false)
	b2 := e.boot(2, 6, 9, true)
	e.convert(b2, float64(base.UnixMilli())+100000) // o boot 2 começa 100 s depois
	e.data(b2, 60, false)
	tr := e.intervalTrip(base.Add(30*time.Second), base.Add(150*time.Second), 9)
	e.progress(9, base)
	e.clock.Advance(10 * time.Second)
	e.calculate(t, e.evaluate(t)[0])
	res := tr.Calc.Result
	if len(res.Parts) != 2 || res.Parts[0].BootID != 1 || res.Parts[1].BootID != 2 {
		t.Fatalf("partes %+v", res.Parts)
	}
	// parte 1: t 30..59 s (eixo 0..29 s); parte 2: t 0..49 s do boot 2 (eixo 70..119 s)
	if res.Parts[0].DataEndTMs != 29000 || res.Parts[1].DataStartTMs != 70000 {
		t.Fatalf("eixo %+v", res.Parts)
	}
	if got := *res.Summary.NoDataS; got != 41 { // da janela da parte 1 (29 s) ao início da parte 2 (70 s)
		t.Fatalf("sem dados %v", got)
	}
	if hasWarning(res, tripanalysis.WarnOverlappingParts) {
		t.Fatal("sem sobreposição")
	}
	// a incerteza passa a cruzar as partes: aviso partes_sobrepostas
	e.db.Groups[b1.BootKey] = []timeline.Group{{Lo: float64(base.UnixMilli()) - 25000, Hi: float64(base.UnixMilli()) + 25000, TCenter: 30000, N: 2}}
	e.db.Groups[b2.BootKey] = []timeline.Group{{Lo: float64(base.UnixMilli()) + 100000 - 25000, Hi: float64(base.UnixMilli()) + 100000 + 25000, TCenter: 30000, N: 2}}
	tr.RecalcPending = true
	if found, err := e.calc.Recalc(context.Background()); err != nil || !found {
		t.Fatalf("%v %v", found, err)
	}
	if !hasWarning(tr.Calc.Result, tripanalysis.WarnOverlappingParts) {
		t.Fatalf("avisos %+v", tr.Calc.Result.Warnings)
	}
}

// Critério 18: entrada boot sem hora funciona com o aviso sem_hora_absoluta.
func TestBootWithoutTime(t *testing.T) {
	e := newEnv(t)
	b := e.boot(1, 1, 5, false)
	e.data(b, 30, false)
	tr := e.bootTrip(1, 5)
	e.progress(5, base)
	e.clock.Advance(10 * time.Second)
	e.calculate(t, e.evaluate(t)[0])
	res := tr.Calc.Result
	if !hasWarning(res, tripanalysis.WarnNoAbsoluteTime) || res.Summary.DistanceKm == nil || res.Summary.ClockOffsetMs != nil {
		t.Fatalf("%+v", res.Warnings)
	}
}

// Viagem vazia (critério 11): aos 20 s fica ready e provisória com sem_leituras; com a geração nula, resolve e recalcula.
func TestEmptyTripAndNilGeneration(t *testing.T) {
	e := newEnv(t)
	tr := e.db.AddTrip(trip.Trip{DongleID: dongleID, Category: "feira", DriverName: "C", Fuel: "diesel", EntryType: "boot", BootID: i32(3),
		TargetSeq: 5, CreatedAt: e.clock.Now()}) // boot_generation nula: o boot não existe
	e.progress(2, base)
	for i := 0; i < 19; i++ {
		e.clock.Advance(time.Second)
		e.progress(2, e.clock.Now())
		if r := e.evaluate(t); len(r) != 0 {
			t.Fatalf("aos %d s: %v", i+1, r)
		}
	}
	e.clock.Advance(time.Second)
	e.calculate(t, e.evaluate(t)[0])
	if tr.Status != trip.StatusReady || !tr.Provisional || !hasWarning(tr.Calc.Result, tripanalysis.WarnNoReadings) || len(tr.Calc.Result.Parts) != 0 {
		t.Fatalf("%+v %+v", tr.Trip, tr.Calc.Result.Warnings)
	}
	// o boot chega ao servidor: o ticker grava a geração e marca o recálculo
	b := e.boot(3, 3, 5, true)
	e.data(b, 30, false)
	e.progress(5, e.clock.Now())
	n, err := e.calc.CheckProvisional(context.Background())
	if err != nil || n != 1 || tr.BootGeneration == nil || !tr.RecalcPending {
		t.Fatalf("n=%d err=%v %+v", n, err, tr.Trip)
	}
	if found, err := e.calc.Recalc(context.Background()); err != nil || !found {
		t.Fatalf("%v %v", found, err)
	}
	if len(tr.Calc.Result.Parts) != 1 || hasWarning(tr.Calc.Result, tripanalysis.WarnNoReadings) {
		t.Fatalf("%+v", tr.Calc.Result.Warnings)
	}
}

// A geração nula também é resolvida pelo avaliador, antes de calcular.
func TestEvaluatorResolvesGeneration(t *testing.T) {
	e := newEnv(t)
	b := e.boot(3, 1, 5, true)
	e.data(b, 30, false)
	tr := e.db.AddTrip(trip.Trip{DongleID: dongleID, Category: "feira", DriverName: "C", Fuel: "diesel", EntryType: "boot", BootID: i32(3),
		TargetSeq: 5, CreatedAt: e.clock.Now()})
	e.progress(5, base)
	e.clock.Advance(10 * time.Second)
	r := e.evaluate(t)
	if len(r) != 1 || tr.BootGeneration == nil {
		t.Fatalf("%v %+v", r, tr.Trip)
	}
	e.calculate(t, r[0])
	if len(tr.Calc.Result.Parts) != 1 {
		t.Fatalf("partes %d", len(tr.Calc.Result.Parts))
	}
}

// Critério 9: erro vira failed e libera o nome; o recálculo com erro mantém o resultado anterior.
func TestFailureAndRecalcFailure(t *testing.T) {
	e := newEnv(t)
	b := e.boot(1, 1, 5, true)
	e.data(b, 30, false)
	bad := e.bootTrip(1, 5)
	bad.Fuel = "lava" // combustível desconhecido: o cálculo falha
	e.progress(5, base)
	e.clock.Advance(10 * time.Second)
	r := e.evaluate(t)
	found, err := e.calc.Calculate(context.Background(), r[0].ID, false)
	bad = e.db.TripByID(bad.ID) // a transação desfeita restaura as linhas da memória
	if !found || !errors.Is(err, tripanalysis.ErrInvalidParams) || bad.Status != trip.StatusFailed || bad.Calc != nil || e.met.trips["failed"] != 1 {
		t.Fatalf("found=%v err=%v %+v", found, err, bad.Trip)
	}
	if len(e.evaluate(t)) != 0 {
		t.Fatal("viagem failed não volta para a fila")
	}

	ok := e.bootTrip(1, 5)
	ok.DriverName = "B"
	e.calculate(t, e.evaluate(t)[0])
	prev := ok.Calc
	ok.RecalcPending, ok.Fuel = true, "lava"
	found, err = e.calc.Recalc(context.Background())
	ok = e.db.TripByID(ok.ID) // a transação desfeita restaura as linhas da memória
	if !found || err == nil || ok.Status != trip.StatusReady || ok.Calc != prev || ok.RecalcPending || ok.Saves != 1 {
		t.Fatalf("found=%v err=%v %+v", found, err, ok.Trip)
	}
	if found, _ := e.calc.Recalc(context.Background()); found {
		t.Fatal("sem tight loop: o recalc_pending foi limpo")
	}
}

type panicSources struct{ port.TripSources }

func (panicSources) FindBoot(context.Context, int32, int32, int32) (trip.Boot, bool, error) {
	panic("boom")
}

type busySources struct{ port.TripSources }

func (busySources) FindBoot(context.Context, int32, int32, int32) (trip.Boot, bool, error) {
	return trip.Boot{}, false, port.ErrDatabaseBusy
}

func wrapSources(f func(port.TripSources) port.TripSources) func(*memory.Database) (port.Repositories, port.Transactor) {
	return func(db *memory.Database) (port.Repositories, port.Transactor) {
		return db.Repositories(), sourcesTx{db: db, f: f}
	}
}

type sourcesTx struct {
	db *memory.Database
	f  func(port.TripSources) port.TripSources
}

func (s sourcesTx) InTransaction(ctx context.Context, fn func(context.Context, port.Repositories) error) error {
	return s.db.InTransaction(ctx, func(ctx context.Context, r port.Repositories) error {
		r.TripSources = s.f(r.TripSources)
		return fn(ctx, r)
	})
}

// Pânico no cálculo é recuperado: a transação é desfeita e a viagem vira failed.
func TestPanicIsRecovered(t *testing.T) {
	e := newEnvWith(t, wrapSources(func(s port.TripSources) port.TripSources { return panicSources{s} }))
	b := e.boot(1, 1, 5, true)
	e.data(b, 30, false)
	tr := e.bootTrip(1, 5)
	e.progress(5, base)
	e.clock.Advance(10 * time.Second)
	found, err := e.calc.Calculate(context.Background(), e.evaluate(t)[0].ID, false)
	tr = e.db.TripByID(tr.ID)
	if !found || err == nil || tr.Status != trip.StatusFailed {
		t.Fatalf("found=%v err=%v %s", found, err, tr.Status)
	}
}

// Banco ocupado não falha a viagem: ela fica na fila para o próximo ciclo.
func TestBusyDatabaseDoesNotFail(t *testing.T) {
	e := newEnvWith(t, wrapSources(func(s port.TripSources) port.TripSources { return busySources{s} }))
	b := e.boot(1, 1, 5, true)
	e.data(b, 30, false)
	tr := e.bootTrip(1, 5)
	e.progress(5, base)
	e.clock.Advance(10 * time.Second)
	_, err := e.calc.Calculate(context.Background(), e.evaluate(t)[0].ID, false)
	tr = e.db.TripByID(tr.ID)
	if !errors.Is(err, port.ErrDatabaseBusy) || tr.Status != trip.StatusWaitingData || e.met.trips["failed"] != 0 {
		t.Fatalf("err=%v %s", err, tr.Status)
	}
}

// Viagem travada por outro processo (ou que mudou de estado): found=false, sem erro.
func TestCalculateUnknownTrip(t *testing.T) {
	e := newEnv(t)
	if found, err := e.calc.Calculate(context.Background(), uuid.New(), false); found || err != nil {
		t.Fatalf("%v %v", found, err)
	}
}

func TestQueue(t *testing.T) {
	e := newEnv(t)
	e.bootTrip(1, 5)
	pend := e.bootTrip(2, 5)
	pend.DriverName = "X"
	done := e.bootTrip(3, 5)
	done.Status, done.RecalcPending = trip.StatusReady, true
	q, err := e.calc.Queue(context.Background())
	if err != nil || q.Waiting != 2 || q.Recalc != 1 {
		t.Fatalf("%+v %v", q, err)
	}
}

// A incerteza da parte vem do grupo de âncoras; boot com hora pelos vizinhos usa a incerteza base.
func TestUncertaintySources(t *testing.T) {
	e := newEnv(t)
	b := e.boot(1, 1, 5, true)
	e.data(b, 30, false)
	delete(e.db.Groups, b.BootKey)
	neighbors := "neighbors"
	b.TimeSource = &neighbors
	tr := e.bootTrip(1, 5)
	e.progress(5, base)
	e.clock.Advance(10 * time.Second)
	e.calculate(t, e.evaluate(t)[0])
	if u := tr.Calc.Result.Summary.ClockUncertaintyMs; u == nil || *u != 100 {
		t.Fatalf("incerteza %v", u)
	}
	// com grupos: meia largura + deriva máxima x distância ao centro, no pior extremo
	e.convert(b, float64(base.UnixMilli()))
	tr.RecalcPending = true
	if _, err := e.calc.Recalc(context.Background()); err != nil {
		t.Fatal(err)
	}
	if u := *tr.Calc.Result.Summary.ClockUncertaintyMs; u != 52 { // 50 + 40 ppm x 30 s = 51,2, arredondado para cima
		t.Fatalf("incerteza %d", u)
	}
}
