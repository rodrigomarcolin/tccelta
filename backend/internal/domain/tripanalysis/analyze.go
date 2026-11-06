package tripanalysis

import (
	"fmt"
	"math"
	"time"
)

// Analyze calcula a análise da viagem. O resultado é determinístico: a mesma entrada, com os mesmos parâmetros e a mesma
// CalculationVersion, gera o mesmo Result.
func Analyze(in Input) (Result, error) {
	if err := in.Entry.Validate(); err != nil {
		return Result{}, err
	}
	if err := in.Params.Validate(); err != nil {
		return Result{}, err
	}
	a := prepare(in)
	if len(a.parts) == 0 {
		return a.empty(), nil
	}
	res := Result{Params: in.Params}
	idle, origin := a.idleRPM()
	res.Gears, res.GearShifts = a.detectGears(idle)
	sum := a.summarize(idle, origin)
	res.SpeedBands = a.speedBands()
	res.Events = a.events(a.positions())
	res.Stops = a.stops(a.positions())
	a.fillEventTotals(&sum, res.Events)
	route := a.route()
	res.Series.Route = route.route
	a.gpsQuality(&sum, route)
	res.Series.RPMDistribution, res.Series.PedalDistribution, res.Series.RPMPedal = a.distributions()
	a.shiftIndicators(&sum, res.GearShifts)
	a.series(&res)
	res.Parts = a.partResults()
	a.clockSummary(&sum)
	res.Summary = sum
	res.Warnings = a.warnings(sum, origin)
	return res, nil
}

// empty é o resultado de uma viagem sem nenhuma leitura: tudo nulo e o aviso sem_leituras.
func (a *analysis) empty() Result {
	res := Result{Params: a.in.Params, Parts: []PartResult{}, SpeedBands: []SpeedBand{}, Events: []Event{}, Stops: []Stop{},
		Gears: []Gear{}, GearShifts: []GearShift{}}
	res.Summary.PIDPeriodsMs = map[string]float64{}
	res.Series = emptySeries()
	res.Warnings = append([]Warning{}, a.in.EntryWarnings...)
	res.Warnings = append(res.Warnings, Warning{Code: WarnNoReadings})
	if a.in.Partial {
		res.Warnings = append(res.Warnings, Warning{Code: WarnPartialData})
	}
	if a.in.Entry.Type == EntryInterval {
		off := a.in.Entry.T0.UnixMilli()
		res.Summary.ClockOffsetMs = &off
	}
	return res
}

func emptySeries() SeriesData {
	return SeriesData{
		Series: Series{TMs: []int64{}, Speed: []*float64{}, RPM: []*float64{}, RPMPerKmh: []*float64{}, Gear: []*int{},
			Pedal: []*float64{}, Accel: []*float64{}, Load: []*float64{}, FuelLH: []*float64{}, FuelCumL: []*float64{},
			DistCumKm: []*float64{}},
		Slow: SlowSeries{Temperature: TemperatureSeries{TMs: []int64{}, C: []*float64{}},
			Tank: TankSeries{TMs: []int64{}, Pct: []*float64{}, MedianPct: []*float64{}}},
		Route: Route{TMs: []int64{}, Lat: []float64{}, Lon: []float64{},
			Segments: RouteSegments{SpeedKmh: []*float64{}, Band: []string{}}},
		RPMDistribution: []RPMBand{}, PedalDistribution: []PedalBand{}, RPMPedal: RPMPedalMatrix{RPMBin: rpmBin, PedalBin: pedalBin, TimeS: [][]float64{}},
	}
}

// partResults lista todas as partes com dados, inclusive as sem motor ligado (janela nula).
func (a *analysis) partResults() []PartResult {
	out := make([]PartResult, 0, len(a.parts))
	for _, p := range a.parts {
		r := PartResult{BootGeneration: p.in.Generation, BootID: p.in.BootID, DataStartTMs: p.dataStart, DataEndTMs: p.dataEnd, Clock: p.clock}
		if p.first >= 0 && !a.neverOn {
			s, e := p.t[p.first], p.t[p.last]
			r.StartTMs, r.EndTMs = &s, &e
		}
		out = append(out, r)
	}
	return out
}

// clockSummary preenche início/fim em hora absoluta e o relógio (spec 02, "Eixo de tempo").
func (a *analysis) clockSummary(s *Summary) {
	at := func(tMs int64) *time.Time {
		var ms float64
		switch a.in.Entry.Type {
		case EntryInterval:
			ms = float64(a.in.Entry.T0.UnixMilli() + tMs)
		default:
			c := a.parts[0].clock
			if c == nil {
				return nil
			}
			ms = c.AMs + (1+c.B)*float64(tMs)
		}
		t := time.UnixMilli(int64(math.Round(ms))).UTC()
		return &t
	}
	if s.StartTMs != nil {
		s.StartedAt, s.EndedAt = at(*s.StartTMs), at(*s.EndTMs)
	}
	switch a.in.Entry.Type {
	case EntryInterval:
		off := a.in.Entry.T0.UnixMilli()
		s.ClockOffsetMs = &off
		var u int32 = -1
		for _, p := range a.parts {
			if p.clock != nil {
				u = max(u, p.clock.UncertaintyMs)
			}
		}
		if u >= 0 {
			s.ClockUncertaintyMs = &u
		}
	default:
		if c := a.parts[0].clock; c != nil {
			off, u := int64(math.Round(c.AMs)), c.UncertaintyMs
			s.ClockOffsetMs, s.ClockUncertaintyMs = &off, &u
		}
	}
}

// warnings monta os avisos na ordem do contrato.
func (a *analysis) warnings(s Summary, idleOrigin string) []Warning {
	w := append([]Warning{}, a.in.EntryWarnings...)
	add := func(code string) { w = append(w, Warning{Code: code}) }
	if a.positionCount() == 0 {
		add(WarnNoGPS)
	}
	if a.in.Entry.Type == EntryBoot && a.parts[0].clock == nil {
		add(WarnNoAbsoluteTime)
	}
	if a.overlap {
		add(WarnOverlappingParts)
	}
	if a.neverOn {
		add(WarnEngineNeverOn)
	}
	if s.FuelCoveragePct != nil && *s.FuelCoveragePct < 90 {
		add(WarnLowMAFCoverage)
	}
	if s.PedalHighS != nil && s.EngineOnS != nil && *s.EngineOnS > 0 && *s.PedalHighS / *s.EngineOnS > a.p.PedalHighWarnFrac {
		add(WarnFuelMayBeUnderrated)
	}
	if idleOrigin == "reserva" {
		add(WarnIdleFallback)
	}
	if a.in.Partial {
		add(WarnPartialData)
	}
	return w
}

func (a *analysis) positionCount() int {
	n := 0
	for _, p := range a.windowed() {
		n += len(p.positionsIn())
	}
	return n
}

// pidName é a chave de periodos_mediana_ms.
func pidName(pid uint8) string { return fmt.Sprintf("0x%02X", pid) }
