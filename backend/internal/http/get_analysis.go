package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"

	"tccelta/backend/internal/domain/trip"
	"tccelta/backend/internal/domain/tripanalysis"
)

// O JSON da análise é o da spec 02, montado com o que o worker gravou: a API só repassa. Os campos numéricos são ponteiros
// sem omitempty, então a chave sempre existe e a ausência é null; as listas saem [] (nunca null).

type analysisJSON struct {
	Version    string            `json:"versao_calculo"`
	Trip       tripJSON          `json:"viagem"`
	Params     json.RawMessage   `json:"parametros"`
	Warnings   json.RawMessage   `json:"avisos"`
	Summary    summaryJSON       `json:"resumo"`
	Driver     driverJSON        `json:"condutor"`
	Engine     engineJSON        `json:"motor"`
	SpeedBands []speedBandJSON   `json:"faixas_velocidade"`
	Route      json.RawMessage   `json:"rota"`
	Stops      []stopJSON        `json:"paradas"`
	Events     []eventJSON       `json:"eventos"`
	Gears      gearsJSON         `json:"marchas"`
	Dist       distributionsJSON `json:"distribuicoes"`
	Series     json.RawMessage   `json:"series"`
	Slow       json.RawMessage   `json:"series_lentas"`
	Quality    qualityJSON       `json:"qualidade"`
}

type tripJSON struct {
	Entry    any        `json:"entrada"`
	Driver   tripDriver `json:"motorista"`
	Category string     `json:"categoria_viagem"`
	StartTMs *int64     `json:"inicio_t_ms"`
	EndTMs   *int64     `json:"fim_t_ms"`
	StartAt  *string    `json:"inicio_em"`
	EndAt    *string    `json:"fim_em"`
	Clock    *clockJSON `json:"relogio"`
	Parts    []partJSON `json:"partes"`
}

type tripDriver struct {
	ID   string `json:"id"`
	Name string `json:"nome"`
}

type clockJSON struct {
	OffsetMs      int64 `json:"deslocamento_ms"`
	UncertaintyMs int32 `json:"incerteza_ms"`
}

type partJSON struct {
	BootGeneration int32  `json:"boot_generation"`
	BootID         int32  `json:"boot_id"`
	DataStartTMs   int64  `json:"dados_inicio_t_ms"`
	DataEndTMs     int64  `json:"dados_fim_t_ms"`
	StartTMs       *int64 `json:"inicio_t_ms"`
	EndTMs         *int64 `json:"fim_t_ms"`
}

type summaryJSON struct {
	DistanceKm      *float64 `json:"distancia_km"`
	DurationS       *float64 `json:"duracao_s"`
	NoDataS         *float64 `json:"sem_dados_s"`
	MovingS         *float64 `json:"em_movimento_s"`
	StoppedS        *float64 `json:"parado_s"`
	AvgSpeedKmh     *float64 `json:"velocidade_media_kmh"`
	MaxSpeedKmh     *float64 `json:"velocidade_maxima_kmh"`
	EngineOnS       *float64 `json:"motor_ligado_s"`
	IdleS           *float64 `json:"marcha_lenta_s"`
	IdlePct         *float64 `json:"marcha_lenta_pct"`
	FuelL           *float64 `json:"combustivel_l"`
	KmPerL          *float64 `json:"km_por_l"`
	NHardAccel      *int32   `json:"n_acel_bruscas"`
	NHardBrake      *int32   `json:"n_fren_bruscas"`
	HardAccelPer100 *float64 `json:"acel_por_100km"`
	HardBrakePer100 *float64 `json:"fren_por_100km"`
}

type driverJSON struct {
	PedalMeanPct     *float64 `json:"pedal_medio_pct"`
	PedalHighS       *float64 `json:"pedal_alto_s"`
	PedalRateP95PctS *float64 `json:"pedal_variacao_p95_pct_s"`
	EngineBrakePct   *float64 `json:"freio_motor_pct"`
	AccelRMSMS2      *float64 `json:"aceleracao_rms_ms2"`
	CruiseS          *float64 `json:"cruzeiro_s"`
}

type engineJSON struct {
	WarmupS     *float64 `json:"aquecimento_s"`
	MaxCoolantC *float64 `json:"temperatura_maxima_c"`
	ColdLoadS   *float64 `json:"esforco_frio_s"`
}

type speedBandJSON struct {
	ID          string   `json:"id"`
	FromKmh     *float64 `json:"de_kmh"`
	ToKmh       *float64 `json:"ate_kmh"`
	TimeS       *float64 `json:"tempo_s"`
	DistanceKm  *float64 `json:"distancia_km"`
	DistancePct *float64 `json:"distancia_pct"`
}

type stopJSON struct {
	StartTMs int64    `json:"t_inicio_ms"`
	Duration *float64 `json:"duracao_s"`
	EngineOn bool     `json:"motor_ligado"`
	Lat      *float64 `json:"lat"`
	Lon      *float64 `json:"lon"`
}

type eventJSON struct {
	Type          string   `json:"tipo"`
	StartTMs      int64    `json:"t_inicio_ms"`
	Duration      *float64 `json:"duracao_s"`
	Peak          *float64 `json:"pico"`
	PeakUnit      string   `json:"pico_unidade"`
	StartSpeedKmh *float64 `json:"velocidade_inicial_kmh"`
	EndSpeedKmh   *float64 `json:"velocidade_final_kmh"`
	Lat           *float64 `json:"lat"`
	Lon           *float64 `json:"lon"`
}

type gearsJSON struct {
	Idle       *idleJSON      `json:"marcha_lenta"`
	Levels     []levelJSON    `json:"patamares"`
	NeutralS   *float64       `json:"ponto_morto_embreagem_s"`
	Shifts     []shiftJSON    `json:"trocas"`
	Upshifts   upshiftsJSON   `json:"subida_rpm"`
	Downshifts downshiftsJSON `json:"reducao_rpm"`
}

type idleJSON struct {
	RPM       float64  `json:"rpm"`
	Origin    *string  `json:"origem"`
	MarginRPM *float64 `json:"margem_rpm"`
}

type levelJSON struct {
	Gear      int      `json:"marcha"`
	RPMPerKmh *float64 `json:"rpm_por_kmh"`
	TimeS     *float64 `json:"tempo_s"`
	TimePct   *float64 `json:"tempo_pct"`
}

type shiftJSON struct {
	TMs       int64    `json:"t_ms"`
	Direction string   `json:"direcao"`
	From      int      `json:"de"`
	To        int      `json:"para"`
	RPMBefore *float64 `json:"rpm_antes"`
	RPMAfter  *float64 `json:"rpm_depois"`
	SpeedKmh  *float64 `json:"velocidade_kmh"`
}

type upshiftsJSON struct {
	Median *float64 `json:"mediana"`
	P25    *float64 `json:"p25"`
	P75    *float64 `json:"p75"`
	N      *int32   `json:"n"`
}

type downshiftsJSON struct {
	Median *float64 `json:"mediana"`
	N      *int32   `json:"n"`
}

type distributionsJSON struct {
	RPM      json.RawMessage `json:"rotacao"`
	Pedal    json.RawMessage `json:"pedal"`
	RPMPedal json.RawMessage `json:"rotacao_pedal"`
}

type qualityJSON struct {
	Readings     int64              `json:"leituras_total"`
	ReadingsPerS *float64           `json:"leituras_por_s"`
	PIDPeriods   map[string]float64 `json:"periodos_mediana_ms"`
	FuelCoverage *float64           `json:"combustivel_cobertura_pct"`
	GPS          gpsJSON            `json:"gps"`
}

type gpsJSON struct {
	PeriodS     *float64 `json:"periodo_mediana_s"`
	CoveragePct *float64 `json:"cobertura_pct"`
	NGaps       *int32   `json:"n_falhas"`
	MaxGapS     *float64 `json:"maior_falha_s"`
	DistanceKm  *float64 `json:"distancia_km"`
	DiffPct     *float64 `json:"diferenca_distancia_pct"`
}

// num converte um float não anulável: NaN e infinito viram null, pois encoding/json falha neles.
func num(v float64) *float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	return &v
}

// raw devolve o jsonb como está; vazio vira null.
func raw(b trip.RawJSON) json.RawMessage {
	if len(bytes.TrimSpace(b)) == 0 {
		return json.RawMessage("null")
	}
	return json.RawMessage(b)
}

// withSteps põe passo_ms e exposicao_passo_ms (colunas próprias) à frente das chaves de series.
func withSteps(s trip.AnalysisSeries) json.RawMessage {
	head := fmt.Sprintf(`{"passo_ms":%d,"exposicao_passo_ms":%d`, s.StepMs, s.ExposureStepMs)
	body := bytes.TrimSpace(s.Series)
	if len(body) < 2 || body[0] != '{' {
		return json.RawMessage(head + "}")
	}
	if inner := bytes.TrimSpace(body[1 : len(body)-1]); len(inner) > 0 {
		return json.RawMessage(head + "," + string(inner) + "}")
	}
	return json.RawMessage(head + "}")
}

func derefOr(p *string, def string) string {
	if p == nil {
		return def
	}
	return *p
}

func analysisFrom(a trip.Analysis) analysisJSON {
	t, s := a.Trip, a.Summary
	out := analysisJSON{
		Version: derefOr(t.CalculationVersion, tripanalysis.CalculationVersion),
		Trip: tripJSON{Entry: entryJSON(t), Driver: tripDriver{ID: t.ID.String(), Name: t.DriverName}, Category: t.Category,
			StartTMs: s.StartTMs, EndTMs: s.EndTMs, StartAt: utcMsPtr(s.StartedAt), EndAt: utcMsPtr(s.EndedAt), Parts: make([]partJSON, len(a.Parts))},
		Params: raw(a.Params), Warnings: raw(a.Warnings),
		Summary: summaryJSON{DistanceKm: s.DistanceKm, DurationS: s.DurationS, NoDataS: s.NoDataS, MovingS: s.MovingS, StoppedS: s.StoppedS,
			AvgSpeedKmh: s.AvgSpeedKmh, MaxSpeedKmh: s.MaxSpeedKmh, EngineOnS: s.EngineOnS, IdleS: s.IdleS, IdlePct: s.IdlePct,
			FuelL: s.FuelL, KmPerL: s.KmPerL, NHardAccel: s.NHardAccel, NHardBrake: s.NHardBrake,
			HardAccelPer100: s.HardAccelPer100Km, HardBrakePer100: s.HardBrakePer100Km},
		Driver: driverJSON{PedalMeanPct: s.PedalMeanPct, PedalHighS: s.PedalHighS, PedalRateP95PctS: s.PedalRateP95PctS,
			EngineBrakePct: s.EngineBrakePct, AccelRMSMS2: s.AccelRMSMS2, CruiseS: s.CruiseS},
		Engine:     engineJSON{WarmupS: s.WarmupS, MaxCoolantC: s.MaxCoolantC, ColdLoadS: s.ColdLoadS},
		SpeedBands: make([]speedBandJSON, len(a.SpeedBands)),
		Route:      raw(a.Series.Route),
		Stops:      make([]stopJSON, len(a.Stops)),
		Events:     make([]eventJSON, len(a.Events)),
		Gears: gearsJSON{NeutralS: s.NeutralClutchS, Levels: make([]levelJSON, len(a.Gears)), Shifts: make([]shiftJSON, len(a.GearShifts)),
			Upshifts:   upshiftsJSON{Median: s.UpshiftRPMMedian, P25: s.UpshiftRPMP25, P75: s.UpshiftRPMP75, N: s.UpshiftN},
			Downshifts: downshiftsJSON{Median: s.DownshiftRPMMedian, N: s.DownshiftN}},
		Dist:   distributionsJSON{RPM: raw(a.Series.RPMDistribution), Pedal: raw(a.Series.PedalDistribution), RPMPedal: raw(a.Series.RPMPedalGrid)},
		Series: withSteps(a.Series), Slow: raw(a.Series.Slow),
		Quality: qualityJSON{Readings: s.NReadings, ReadingsPerS: s.ReadingsPerS, PIDPeriods: s.PIDPeriodsMs, FuelCoverage: s.FuelCoveragePct,
			GPS: gpsJSON{PeriodS: s.GPSPeriodS, CoveragePct: s.GPSCoveragePct, NGaps: s.GPSNGaps, MaxGapS: s.GPSMaxGapS,
				DistanceKm: s.GPSDistanceKm, DiffPct: s.GPSDistanceDiffPct}},
	}
	if out.Quality.PIDPeriods == nil {
		out.Quality.PIDPeriods = map[string]float64{}
	}
	if s.ClockOffsetMs != nil && s.ClockUncertaintyMs != nil {
		out.Trip.Clock = &clockJSON{OffsetMs: *s.ClockOffsetMs, UncertaintyMs: *s.ClockUncertaintyMs}
	}
	if s.IdleRPM != nil {
		out.Gears.Idle = &idleJSON{RPM: *s.IdleRPM, Origin: s.IdleRPMOrigin, MarginRPM: s.IdleMarginRPM}
	}
	for i, p := range a.Parts {
		out.Trip.Parts[i] = partJSON{p.BootGeneration, p.BootID, p.DataStartTMs, p.DataEndTMs, p.StartTMs, p.EndTMs}
	}
	for i, b := range a.SpeedBands {
		out.SpeedBands[i] = speedBandJSON{ID: b.ID, FromKmh: num(b.FromKmh), ToKmh: b.ToKmh, TimeS: num(b.TimeS),
			DistanceKm: num(b.DistanceKm), DistancePct: b.DistancePct}
	}
	for i, x := range a.Stops {
		out.Stops[i] = stopJSON{StartTMs: x.StartTMs, Duration: num(x.DurationS), EngineOn: x.EngineOn, Lat: x.Lat, Lon: x.Lon}
	}
	for i, e := range a.Events {
		out.Events[i] = eventJSON{Type: e.Type, StartTMs: e.StartTMs, Duration: num(e.DurationS), Peak: num(e.Peak), PeakUnit: e.PeakUnit,
			StartSpeedKmh: e.StartSpeedKmh, EndSpeedKmh: e.EndSpeedKmh, Lat: e.Lat, Lon: e.Lon}
	}
	for i, g := range a.Gears {
		out.Gears.Levels[i] = levelJSON{Gear: g.Gear, RPMPerKmh: num(g.RPMPerKmh), TimeS: num(g.TimeS), TimePct: num(g.TimePct)}
	}
	for i, x := range a.GearShifts {
		out.Gears.Shifts[i] = shiftJSON{TMs: x.TMs, Direction: x.Direction, From: x.From, To: x.To, RPMBefore: num(x.RPMBefore),
			RPMAfter: num(x.RPMAfter), SpeedKmh: num(x.SpeedKmh)}
	}
	return out
}

// getAnalysis: GET /v1/viagens/{id}/analise. Os parâmetros de consulta são ignorados.
func (d *Deps) getAnalysis(w http.ResponseWriter, r *http.Request) {
	id, ok := tripIDFromPath(w, r)
	if !ok {
		return
	}
	a, err := d.GetAnalysis.Run(r.Context(), id)
	if err != nil {
		d.err(w, err)
		return
	}
	body, err := json.Marshal(analysisFrom(a))
	if err != nil { // um valor não finito que escapou das conversões: melhor 500 que corpo cortado
		d.err(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(body)
}
