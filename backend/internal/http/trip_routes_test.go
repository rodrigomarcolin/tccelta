package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"tccelta/backend/internal/config"
	"tccelta/backend/internal/domain/dongle"
	"tccelta/backend/internal/domain/trip"
	"tccelta/backend/internal/domain/tripanalysis"
	"tccelta/backend/internal/http/ratelimit"
	"tccelta/backend/internal/port"
	"tccelta/backend/internal/port/porttest"
)

type fakeCreateTrip struct {
	in  trip.Input
	out trip.Trip
	err error
}

func (f *fakeCreateTrip) Run(_ context.Context, in trip.Input) (trip.Trip, error) {
	f.in = in
	return f.out, f.err
}

type fakeGetTrip struct {
	id  uuid.UUID
	out trip.Trip
	err error
}

func (f *fakeGetTrip) Run(_ context.Context, id uuid.UUID) (trip.Trip, error) {
	f.id = id
	return f.out, f.err
}

type fakeGetAnalysis struct {
	out trip.Analysis
	err error
}

func (f *fakeGetAnalysis) Run(context.Context, uuid.UUID) (trip.Analysis, error) { return f.out, f.err }

// recorder guarda as rotas e os estados das métricas HTTP.
type recorder struct {
	porttest.Nop
	routes     []string
	rejections []string
}

func (r *recorder) Request(route string, _ int, _ time.Duration) { r.routes = append(r.routes, route) }
func (r *recorder) Rejection(reason string)                      { r.rejections = append(r.rejections, reason) }

type tripEnv struct {
	d        *Deps
	create   *fakeCreateTrip
	get      *fakeGetTrip
	analysis *fakeGetAnalysis
	met      *recorder
}

func newTripEnv(t *testing.T, env map[string]string) *tripEnv {
	t.Helper()
	cfg, err := config.LoadFrom(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	e := &tripEnv{create: &fakeCreateTrip{}, get: &fakeGetTrip{}, analysis: &fakeGetAnalysis{}, met: &recorder{}}
	e.d = &Deps{Cfg: cfg, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Met: e.met, CreateTrip: e.create, GetTrip: e.get,
		GetAnalysis: e.analysis, AuthenticateClient: acceptAnyClient{}, Limiter: ratelimit.NewLocal(), Semaphore: ratelimit.NewConcurrency(1, 0)}
	return e
}

// acceptAnyClient aceita qualquer Bearer: os testes das rotas de viagem não exercitam a autenticação (ver token_test.go).
type acceptAnyClient struct{}

func (acceptAnyClient) Run(context.Context, string) (port.ClientSession, error) {
	return port.ClientSession{}, nil
}

// router devolve o roteador com um Bearer qualquer nas requisições que não trazem Authorization.
func (e *tripEnv) router() http.Handler {
	h := e.d.Router()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			r.Header.Set("Authorization", "Bearer teste")
		}
		h.ServeHTTP(w, r)
	})
}

var tripID = uuid.MustParse("6f1c1f7e-3f0a-4a53-9d51-0c1d7a1b2c3d")

func jsonHdr() map[string]string { return map[string]string{"Content-Type": "application/json"} }

const validBody = `{"motorista":{"nome":"João"},"categoria_viagem":"feira","combustivel":"gasolina_e30",` +
	`"entrada":{"tipo":"boot","dongle_id":1,"boot_id":8}}`

func decode(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("%v: %s", err, w.Body.String())
	}
	return m
}

func TestCreateTripAccepted(t *testing.T) {
	e := newTripEnv(t, nil)
	e.create.out = trip.Trip{ID: tripID, Status: trip.StatusPending}
	w := req(e.router(), "POST", "/v1/viagens", []byte(validBody), jsonHdr())
	if w.Code != 202 || w.Header().Get("Location") != "/v1/viagens/"+tripID.String() || w.Header().Get("X-Request-Id") == "" {
		t.Fatal(w.Code, w.Header(), w.Body.String())
	}
	m := decode(t, w)
	if m["viagem_id"] != tripID.String() || m["status"] != "pendente" {
		t.Fatalf("%v", m)
	}
	in := e.create.in
	if in.DriverName != "João" || in.Category != "feira" || in.Fuel != "gasolina_e30" || in.EntryType != "boot" ||
		*in.DongleID != 1 || *in.BootID != 8 || in.BootGeneration != nil || in.T0 != nil {
		t.Fatalf("%+v", in)
	}
	body := `{"motorista":{"nome":"Ana"},"categoria_viagem":"x","combustivel":"diesel","entrada":{"tipo":"intervalo","dongle_id":2,` +
		`"t0":"2026-10-28T08:00:00-03:00","t1":"2026-10-28T09:00:00Z"}}`
	if w := req(e.router(), "POST", "/v1/viagens", []byte(body), map[string]string{"Content-Type": "application/json; charset=utf-8"}); w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	if *e.create.in.T0 != "2026-10-28T08:00:00-03:00" || *e.create.in.T1 != "2026-10-28T09:00:00Z" || e.create.in.BootID != nil {
		t.Fatalf("%+v", e.create.in)
	}
}

func TestCreateTripTransportErrors(t *testing.T) {
	e := newTripEnv(t, nil)
	h := e.router()
	big := `{"motorista":{"nome":"` + strings.Repeat("a", maxTripBody) + `"}}`
	padded := validBody + strings.Repeat(" ", maxTripBody) // objeto completo e espaços até passar do limite
	cases := []struct {
		name   string
		body   string
		hdr    map[string]string
		status int
		code   string
	}{
		{"text/plain", validBody, map[string]string{"Content-Type": "text/plain"}, 415, "tipo_conteudo"},
		{"sem Content-Type", validBody, nil, 415, "tipo_conteudo"},
		{"JSON inválido", `{"motorista":`, jsonHdr(), 400, "corpo_malformado"},
		{"corpo vazio", ``, jsonHdr(), 400, "corpo_malformado"},
		{"campo desconhecido", `{"motorista":{"nome":"a"},"extra":1}`, jsonHdr(), 400, "corpo_malformado"},
		{"campo desconhecido na entrada", `{"entrada":{"tipo":"boot","x":1}}`, jsonHdr(), 400, "corpo_malformado"},
		{"tipo errado", `{"entrada":{"dongle_id":"1"}}`, jsonHdr(), 400, "corpo_malformado"},
		{"inteiro fracionário", `{"entrada":{"dongle_id":1.5}}`, jsonHdr(), 400, "corpo_malformado"},
		{"dados depois do objeto", validBody + `{}`, jsonHdr(), 400, "corpo_malformado"},
		{"corpo de 4097 bytes", big, jsonHdr(), 413, "corpo_grande"},
		{"objeto válido seguido de espaços acima do limite", padded, jsonHdr(), 413, "corpo_grande"},
	}
	for _, c := range cases {
		w := req(h, "POST", "/v1/viagens", []byte(c.body), c.hdr)
		if w.Code != c.status || code(w) != c.code || w.Header().Get("Content-Type") != "application/problem+json" {
			t.Errorf("%s: %d %s", c.name, w.Code, w.Body.String())
		}
	}
	if e.create.in.DriverName != "" {
		t.Fatal("o caso de uso não deve rodar com corpo ruim")
	}
	// o limite é de 4096 bytes: exatamente 4096 ainda passa
	exact := validBody + strings.Repeat(" ", maxTripBody-len(validBody))
	e.create.out = trip.Trip{ID: tripID, Status: trip.StatusPending}
	if w := req(h, "POST", "/v1/viagens", []byte(exact), jsonHdr()); w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestCreateTripUseCaseErrors(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
		detail string
		retry  bool
	}{
		{trip.FieldError{Field: "motorista.nome", Reason: "obrigatório"}, 400, "entrada_invalida", "motorista.nome", false},
		{trip.ErrDriverRepeated, 409, "motorista_repetido", "", false},
		{trip.ErrBootNotFound, 404, "boot_desconhecido", "", false},
		{dongle.ErrUnknown, 404, "dongle_desconhecido", "", false},
		{port.ErrDatabaseBusy, 503, "banco_ocupado", "", true},
		{errors.New("boom"), 500, "erro_interno", "", false},
	}
	for _, c := range cases {
		e := newTripEnv(t, nil)
		e.create.err = c.err
		w := req(e.router(), "POST", "/v1/viagens", []byte(validBody), jsonHdr())
		m := decode(t, w)
		if w.Code != c.status || code(w) != c.code || !strings.Contains(m["detail"].(string), c.detail) || (w.Header().Get("Retry-After") != "") != c.retry {
			t.Errorf("%v: %d %s", c.err, w.Code, w.Body.String())
		}
	}
}

func TestGetTripStatus(t *testing.T) {
	gen := int32(0)
	boot := int32(8)
	created := time.Date(2026, 10, 28, 17, 3, 0, 123_000_000, time.UTC)
	calc := created.Add(12*time.Second + 500*time.Millisecond)
	base := trip.Trip{ID: tripID, DriverName: "João", Category: "feira", Fuel: "gasolina_e30", DongleID: 1, EntryType: trip.EntryTypeBoot,
		BootGeneration: &gen, BootID: &boot, CreatedAt: created}
	for dbStatus, api := range map[string]string{trip.StatusPending: "pendente", trip.StatusWaitingData: "aguardando_dados",
		trip.StatusReady: "pronta", trip.StatusFailed: "falhou"} {
		e := newTripEnv(t, nil)
		tr := base
		tr.Status, tr.Provisional = dbStatus, dbStatus == trip.StatusReady
		if dbStatus == trip.StatusReady {
			tr.CalculatedAt = &calc
		}
		e.get.out = tr
		w := req(e.router(), "GET", "/v1/viagens/"+tripID.String(), nil, nil)
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || e.get.id != tripID {
			t.Fatal(w.Code, w.Header(), w.Body.String())
		}
		m := decode(t, w)
		if m["status"] != api || m["provisoria"] != (dbStatus == trip.StatusReady) || m["viagem_id"] != tripID.String() ||
			m["categoria_viagem"] != "feira" || m["combustivel"] != "gasolina_e30" || m["criada_em"] != "2026-10-28T17:03:00.123Z" ||
			m["motorista"].(map[string]any)["nome"] != "João" {
			t.Fatalf("%s: %v", dbStatus, m)
		}
		ent := m["entrada"].(map[string]any)
		if ent["tipo"] != "boot" || ent["dongle_id"] != 1.0 || ent["boot_id"] != 8.0 || ent["boot_generation"] != 0.0 || len(ent) != 4 {
			t.Fatalf("%v", ent)
		}
		if dbStatus == trip.StatusReady {
			if m["calculada_em"] != "2026-10-28T17:03:12.623Z" {
				t.Fatalf("%v", m["calculada_em"])
			}
		} else if v, ok := m["calculada_em"]; !ok || v != nil {
			t.Fatalf("calculada_em deve ser null: %v", m)
		}
	}
}

func TestGetTripStatusNullGenerationAndInterval(t *testing.T) {
	e := newTripEnv(t, nil)
	boot := int32(8)
	e.get.out = trip.Trip{ID: tripID, Status: trip.StatusPending, EntryType: trip.EntryTypeBoot, BootID: &boot, DongleID: 1}
	ent := decode(t, req(e.router(), "GET", "/v1/viagens/"+tripID.String(), nil, nil))["entrada"].(map[string]any)
	if v, ok := ent["boot_generation"]; !ok || v != nil {
		t.Fatalf("boot_generation deve ser null: %v", ent)
	}
	loc := time.FixedZone("x", -3*3600)
	t0 := time.Date(2026, 10, 28, 8, 0, 0, 0, loc)
	t1 := t0.Add(time.Hour)
	e.get.out = trip.Trip{ID: tripID, Status: trip.StatusPending, EntryType: trip.EntryTypeInterval, IntervalStart: &t0, IntervalEnd: &t1, DongleID: 2}
	ent = decode(t, req(e.router(), "GET", "/v1/viagens/"+tripID.String(), nil, nil))["entrada"].(map[string]any)
	if ent["tipo"] != "intervalo" || ent["t0"] != "2026-10-28T11:00:00.000Z" || ent["t1"] != "2026-10-28T12:00:00.000Z" || len(ent) != 4 {
		t.Fatalf("%v", ent)
	}
}

func TestTripIDValidation(t *testing.T) {
	e := newTripEnv(t, nil)
	h := e.router()
	for _, id := range []string{"abc", "6f1c1f7e3f0a4a539d510c1d7a1b2c3d", "urn:uuid:" + tripID.String(), "%7B" + tripID.String() + "%7D"} {
		for _, suffix := range []string{"", "/analise"} {
			if w := req(h, "GET", "/v1/viagens/"+id+suffix, nil, nil); w.Code != 404 || code(w) != "viagem_desconhecida" {
				t.Errorf("%s%s: %d %s", id, suffix, w.Code, w.Body.String())
			}
		}
	}
	e.get.err = trip.ErrNotFound
	if w := req(h, "GET", "/v1/viagens/"+uuid.NewString(), nil, nil); w.Code != 404 || code(w) != "viagem_desconhecida" {
		t.Fatal(w.Code, w.Body.String())
	}
	e.analysis.err = trip.ErrNotFound
	if w := req(h, "GET", "/v1/viagens/"+uuid.NewString()+"/analise", nil, nil); w.Code != 404 || code(w) != "viagem_desconhecida" {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestAnalysisNotReady(t *testing.T) {
	for dbStatus, api := range map[string]string{trip.StatusPending: "pendente", trip.StatusWaitingData: "aguardando_dados", trip.StatusFailed: "falhou"} {
		e := newTripEnv(t, nil)
		e.analysis.err = trip.NotReadyError{Status: dbStatus}
		w := req(e.router(), "GET", "/v1/viagens/"+tripID.String()+"/analise", nil, nil)
		m := decode(t, w)
		if w.Code != 409 || m["codigo"] != "analise_indisponivel" || m["viagem_status"] != api || m["status"] != 409.0 {
			t.Errorf("%s: %d %s", dbStatus, w.Code, w.Body.String())
		}
	}
}

func f(v float64) *float64 { return &v }
func i32(v int32) *int32   { return &v }

func readyAnalysis() trip.Analysis {
	gen, boot := int32(0), int32(8)
	version := "1"
	start, end := int64(33481), int64(268453)
	at := time.Date(2026, 10, 28, 17, 3, 22, 0, time.UTC)
	offset, unc := int64(1793206969000), int32(180)
	origin := "estimada"
	return trip.Analysis{
		Trip: trip.Trip{ID: tripID, DriverName: "João", Category: "feira", Fuel: "gasolina_e30", DongleID: 1, EntryType: trip.EntryTypeBoot,
			BootGeneration: &gen, BootID: &boot, Status: trip.StatusReady, CalculationVersion: &version},
		Parts: []tripanalysis.PartResult{{BootGeneration: 0, BootID: 8, DataStartTMs: 881, DataEndTMs: 268453, StartTMs: &start, EndTMs: &end}},
		Summary: tripanalysis.Summary{StartTMs: &start, EndTMs: &end, StartedAt: &at, EndedAt: &at, ClockOffsetMs: &offset, ClockUncertaintyMs: &unc,
			DistanceKm: f(0.30000000000000004), DurationS: f(235), AvgSpeedKmh: f(63), NHardAccel: i32(2), IdleRPM: f(640), IdleRPMOrigin: &origin,
			IdleMarginRPM: f(150), UpshiftRPMMedian: f(2649), UpshiftN: i32(14), NReadings: 8202,
			PIDPeriodsMs: map[string]float64{"0x0C": 130}, GPSCoveragePct: f(98)},
		SpeedBands: []tripanalysis.SpeedBand{{ID: "parado", FromKmh: 0, ToKmh: f(3), TimeS: 23}, {ID: "acima_110", FromKmh: 110, TimeS: 11, DistanceKm: 0.35, DistancePct: f(10)}},
		Events:     []tripanalysis.Event{{Type: "frenagem_brusca", StartTMs: 81600, DurationS: 2, Peak: -6.8, PeakUnit: "m/s2", StartSpeedKmh: f(77), Lat: f(-25.43), Lon: f(-49.27)}},
		Stops:      []tripanalysis.Stop{{StartTMs: 120000, DurationS: 12, EngineOn: true}},
		Gears:      []tripanalysis.Gear{{Gear: 1, RPMPerKmh: 66.2, TimeS: 21, TimePct: 13}},
		GearShifts: []tripanalysis.GearShift{{TMs: 65300, Direction: "subida", From: 1, To: 2, RPMBefore: 2742, RPMAfter: math.Inf(1), SpeedKmh: 41}},
		Warnings:   trip.RawJSON(`[{"codigo": "consumo_pode_estar_subestimado"}]`),
		Params:     trip.RawJSON(`{"combustivel": "gasolina_e30", "afr": 13.0}`),
		Series: trip.AnalysisSeries{StepMs: 150, ExposureStepMs: 300, Series: trip.RawJSON(`{"t_ms": [1, 2], "rpm": [640, null]}`),
			Slow: trip.RawJSON(`{"temperatura": {"t_ms": [], "c": []}}`), Route: trip.RawJSON(`{"t_ms": [1], "lat": [-25.4], "lon": [-49.2]}`),
			RPMDistribution: trip.RawJSON(`[]`), PedalDistribution: trip.RawJSON(`[]`), RPMPedalGrid: trip.RawJSON(`{"rpm_bin": 500}`)},
	}
}

func TestAnalysisReady(t *testing.T) {
	e := newTripEnv(t, nil)
	e.analysis.out = readyAnalysis()
	w := req(e.router(), "GET", "/v1/viagens/"+tripID.String()+"/analise?de_t_ms=5&x=1", nil, nil)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Type") != "application/json" {
		t.Fatal(w.Code, w.Header(), w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"distancia_km":0.30000000000000004`) {
		t.Fatalf("o float deve sair sem arredondar: %s", w.Body.String())
	}
	m := decode(t, w)
	for _, k := range []string{"versao_calculo", "viagem", "parametros", "avisos", "resumo", "condutor", "motor", "faixas_velocidade", "rota", "paradas",
		"eventos", "marchas", "distribuicoes", "series", "series_lentas", "qualidade"} {
		if _, ok := m[k]; !ok {
			t.Errorf("falta a chave %s", k)
		}
	}
	v := m["viagem"].(map[string]any)
	if v["motorista"].(map[string]any)["id"] != tripID.String() || v["motorista"].(map[string]any)["nome"] != "João" || v["categoria_viagem"] != "feira" ||
		v["inicio_em"] != "2026-10-28T17:03:22.000Z" || v["relogio"].(map[string]any)["deslocamento_ms"] != 1793206969000.0 ||
		v["entrada"].(map[string]any)["boot_generation"] != 0.0 || len(v["partes"].([]any)) != 1 {
		t.Fatalf("%v", v)
	}
	if m["versao_calculo"] != "1" || m["parametros"].(map[string]any)["afr"] != 13.0 || m["avisos"].([]any)[0].(map[string]any)["codigo"] != "consumo_pode_estar_subestimado" {
		t.Fatalf("%v", m)
	}
	s := m["series"].(map[string]any)
	if s["passo_ms"] != 150.0 || s["exposicao_passo_ms"] != 300.0 || len(s["t_ms"].([]any)) != 2 || s["rpm"].([]any)[1] != nil {
		t.Fatalf("%v", s)
	}
	res := m["resumo"].(map[string]any)
	if res["distancia_km"] != 0.30000000000000004 || res["n_acel_bruscas"] != 2.0 || res["em_movimento_s"] != nil || res["acel_por_100km"] != nil {
		t.Fatalf("%v", res)
	}
	if cond := m["condutor"].(map[string]any); cond["pedal_medio_pct"] != nil || len(cond) != 6 {
		t.Fatalf("ausência é null, com a chave presente: %v", cond)
	}
	mar := m["marchas"].(map[string]any)
	if mar["marcha_lenta"].(map[string]any)["origem"] != "estimada" || mar["subida_rpm"].(map[string]any)["n"] != 14.0 ||
		mar["trocas"].([]any)[0].(map[string]any)["rpm_depois"] != nil || mar["reducao_rpm"].(map[string]any)["mediana"] != nil ||
		mar["patamares"].([]any)[0].(map[string]any)["rpm_por_kmh"] != 66.2 {
		t.Fatalf("%v", mar)
	}
	faixas := m["faixas_velocidade"].([]any)
	if faixas[1].(map[string]any)["ate_kmh"] != nil || faixas[0].(map[string]any)["distancia_pct"] != nil {
		t.Fatalf("%v", faixas)
	}
	ev := m["eventos"].([]any)[0].(map[string]any)
	if ev["pico_unidade"] != "m/s2" || ev["velocidade_final_kmh"] != nil || ev["lat"] != -25.43 {
		t.Fatalf("%v", ev)
	}
	if q := m["qualidade"].(map[string]any); q["leituras_total"] != 8202.0 || q["periodos_mediana_ms"].(map[string]any)["0x0C"] != 130.0 ||
		q["gps"].(map[string]any)["cobertura_pct"] != 98.0 || q["gps"].(map[string]any)["n_falhas"] != nil {
		t.Fatalf("%v", q)
	}
	if m["rota"].(map[string]any)["lat"].([]any)[0] != -25.4 {
		t.Fatalf("%v", m["rota"])
	}
}

func TestAnalysisEmptyTrip(t *testing.T) {
	e := newTripEnv(t, nil)
	gen, boot := int32(0), int32(8)
	e.analysis.out = trip.Analysis{
		Trip:     trip.Trip{ID: tripID, DriverName: "João", Category: "feira", EntryType: trip.EntryTypeBoot, BootGeneration: &gen, BootID: &boot, Status: trip.StatusReady},
		Warnings: trip.RawJSON(`[{"codigo":"sem_leituras"}]`), Params: trip.RawJSON(`{}`),
		Series: trip.AnalysisSeries{StepMs: 150, ExposureStepMs: 150, Series: trip.RawJSON(`{}`), Slow: trip.RawJSON(`{}`), RPMDistribution: trip.RawJSON(`[]`),
			PedalDistribution: trip.RawJSON(`[]`), RPMPedalGrid: trip.RawJSON(`{}`)},
	}
	w := req(e.router(), "GET", "/v1/viagens/"+tripID.String()+"/analise", nil, nil)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	m := decode(t, w)
	if v, ok := m["rota"]; !ok || v != nil {
		t.Fatalf("rota deve ser null: %v", m["rota"])
	}
	if m["resumo"].(map[string]any)["distancia_km"] != nil || m["viagem"].(map[string]any)["relogio"] != nil || m["viagem"].(map[string]any)["inicio_em"] != nil ||
		m["marchas"].(map[string]any)["marcha_lenta"] != nil || m["versao_calculo"] != tripanalysis.CalculationVersion {
		t.Fatalf("%v", m)
	}
	for _, path := range []struct {
		obj map[string]any
		key string
	}{{m, "faixas_velocidade"}, {m, "eventos"}, {m, "paradas"}, {m["viagem"].(map[string]any), "partes"},
		{m["marchas"].(map[string]any), "trocas"}, {m["marchas"].(map[string]any), "patamares"}} {
		if l, ok := path.obj[path.key].([]any); !ok || len(l) != 0 {
			t.Errorf("%s deve ser []: %v", path.key, path.obj[path.key])
		}
	}
	if w := w.Body.String(); !strings.Contains(w, `"avisos":[{"codigo":"sem_leituras"}]`) || !strings.Contains(w, `"series":{"passo_ms":150,"exposicao_passo_ms":150}`) {
		t.Fatalf("%s", w)
	}
}

func TestWithStepsEdgeCases(t *testing.T) {
	cases := map[string]string{
		``:               `{"passo_ms":1,"exposicao_passo_ms":2}`,
		`null`:           `{"passo_ms":1,"exposicao_passo_ms":2}`,
		`{}`:             `{"passo_ms":1,"exposicao_passo_ms":2}`,
		` { "a": [1] } `: `{"passo_ms":1,"exposicao_passo_ms":2,"a": [1]}`,
	}
	for in, want := range cases {
		if got := string(withSteps(trip.AnalysisSeries{StepMs: 1, ExposureStepMs: 2, Series: trip.RawJSON(in)})); got != want {
			t.Errorf("%q: %s", in, got)
		}
	}
	if string(raw(nil)) != "null" || string(raw(trip.RawJSON(" "))) != "null" || derefOr(nil, "x") != "x" {
		t.Fatal("raw/derefOr")
	}
}

func TestAnalysisInternalError(t *testing.T) {
	e := newTripEnv(t, nil)
	e.analysis.err = port.ErrDatabaseBusy
	if w := req(e.router(), "GET", "/v1/viagens/"+tripID.String()+"/analise", nil, nil); w.Code != 503 || code(w) != "banco_ocupado" {
		t.Fatal(w.Code, w.Body.String())
	}
	e.analysis.err = nil
	a := readyAnalysis()
	a.Params = trip.RawJSON(`{invalido`) // jsonb corrompido: melhor 500 que corpo cortado
	e.analysis.out = a
	if w := req(e.router(), "GET", "/v1/viagens/"+tripID.String()+"/analise", nil, nil); w.Code != 500 || code(w) != "erro_interno" {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestTripMetricsRoutesAndRequestID(t *testing.T) {
	e := newTripEnv(t, nil)
	e.create.out = trip.Trip{ID: tripID, Status: trip.StatusPending}
	e.get.out = trip.Trip{ID: tripID, Status: trip.StatusPending}
	e.analysis.out = readyAnalysis()
	h := e.router()
	r1 := req(h, "POST", "/v1/viagens", []byte(validBody), jsonHdr())
	r2 := req(h, "GET", "/v1/viagens/"+tripID.String(), nil, map[string]string{"X-Request-Id": "abc-123"})
	r3 := req(h, "GET", "/v1/viagens/"+tripID.String()+"/analise", nil, nil)
	if strings.Join(e.met.routes, ",") != "trip_create,trip_status,trip_analysis" {
		t.Fatalf("%v", e.met.routes)
	}
	if r1.Header().Get("X-Request-Id") == "" || r2.Header().Get("X-Request-Id") != "abc-123" || r3.Header().Get("X-Request-Id") == "" {
		t.Fatal("X-Request-Id")
	}
	for _, w := range []*httptest.ResponseRecorder{r1, r2, r3} {
		if w.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Fatal("a API não envia cabeçalhos CORS")
		}
	}
}

func TestTripRateLimitScope(t *testing.T) {
	e := newTripEnv(t, map[string]string{"LIMITE_VIAGEM_IP_MIN": "600", "LIMITE_IP_MIN": "2"})
	e.get.out = trip.Trip{ID: tripID, Status: trip.StatusPending}
	h := e.router()
	// 130 consultas de um IP passam, mesmo com o limite geral (LIMITE_IP_MIN) em 2: os escopos são separados
	for i := 0; i < 130; i++ {
		if w := req(h, "GET", "/v1/viagens/"+tripID.String(), nil, nil); w.Code != 200 {
			t.Fatalf("consulta %d: %d", i, w.Code)
		}
	}
	// e o limite geral continua valendo nas rotas de dongle
	for i := 0; i < 2; i++ {
		req(h, "POST", "/v1/dongles/zz/desafio", nil, nil)
	}
	if w := req(h, "POST", "/v1/dongles/zz/desafio", nil, nil); w.Code != 429 {
		t.Fatalf("limite geral: %d", w.Code)
	}

	e2 := newTripEnv(t, map[string]string{"LIMITE_VIAGEM_IP_MIN": "5"})
	e2.get.out = trip.Trip{ID: tripID, Status: trip.StatusPending}
	h2 := e2.router()
	for i := 0; i < 5; i++ {
		if w := req(h2, "GET", "/v1/viagens/"+tripID.String(), nil, nil); w.Code != 200 {
			t.Fatalf("consulta %d: %d", i, w.Code)
		}
	}
	w := req(h2, "GET", "/v1/viagens/"+tripID.String(), nil, nil)
	if w.Code != 429 || code(w) != "limite_taxa" || w.Header().Get("Retry-After") == "" || len(e2.met.rejections) != 1 || e2.met.rejections[0] != "rate_limit" {
		t.Fatal(w.Code, w.Body.String(), e2.met.rejections)
	}
	// o contador vale também para o POST e para a análise (mesmo escopo)
	if w := req(h2, "POST", "/v1/viagens", []byte(validBody), jsonHdr()); w.Code != 429 {
		t.Fatal(w.Code)
	}
}

func TestDocumentedPanicRecovery(t *testing.T) {
	e := newTripEnv(t, nil)
	e.d.GetTrip = nil // nil interface: pânico no handler
	w := req(e.router(), "GET", "/v1/viagens/"+tripID.String(), nil, nil)
	if w.Code != 500 || code(w) != "erro_interno" {
		t.Fatal(w.Code, w.Body.String())
	}
}
