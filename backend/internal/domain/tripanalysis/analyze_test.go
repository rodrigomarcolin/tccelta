package tripanalysis

import (
	"errors"
	"math"
	"reflect"
	"testing"
	"time"
)

// Critério 1: fronteiras das faixas.
func TestBandID(t *testing.T) {
	for _, c := range []struct {
		v    float64
		want string
	}{{0, "parado"}, {2.9, "parado"}, {3, "ate_30"}, {29.9, "ate_30"}, {30, "30_60"}, {59.9, "30_60"}, {60, "60_80"},
		{80, "80_110"}, {109.9, "80_110"}, {110, "acima_110"}, {200, "acima_110"}} {
		if got := BandID(c.v, 3); got != c.want {
			t.Errorf("BandID(%v) = %s, esperado %s", c.v, got, c.want)
		}
	}
}

// Critério 2 e resumo básico: velocidade constante de 36 km/h por 50 s depois de 10 s com o motor desligado (critério 6).
func TestSummaryBasicsAndWindow(t *testing.T) {
	p := PartInput{BootID: 1, Readings: readings(
		gen(pidRPM, 0, 59000, 1000, func(t int64) float64 {
			if t < 10000 {
				return 0
			}
			return 1500
		}),
		gen(pidSpeed, 0, 59000, 1000, konst(36)))}
	r := mustAnalyze(t, input(p))
	s := r.Summary
	if r.Parts[0].DataStartTMs != 0 || r.Parts[0].StartTMs == nil || *r.Parts[0].StartTMs != 10000 || *r.Parts[0].EndTMs != 59000 {
		t.Fatalf("parte %+v", r.Parts[0])
	}
	near(t, "duração", s.DurationS, 49, 1e-9)
	near(t, "sem dados", s.NoDataS, 0, 1e-9)
	near(t, "distância", s.DistanceKm, 0.5, 1e-9) // 50 pontos x 10 m/s x 1 s
	near(t, "movimento", s.MovingS, 50, 1e-9)
	near(t, "parado", s.StoppedS, 0, 1e-9)
	near(t, "média", s.AvgSpeedKmh, 36, 1e-9)
	near(t, "máxima", s.MaxSpeedKmh, 36, 1e-9)
	near(t, "motor", s.EngineOnS, 50, 1e-9)
	near(t, "marcha lenta", s.IdleS, 0, 1e-9)
	near(t, "marcha lenta %", s.IdlePct, 0, 1e-9)
	if *s.StartTMs != 10000 || *s.EndTMs != 59000 {
		t.Fatalf("janela %d-%d", *s.StartTMs, *s.EndTMs)
	}
	isNil(t, "combustível sem MAF", s.FuelL)
	isNil(t, "cruzeiro", ptrIf(s.CruiseS, false)) // 36 km/h < 40: sem trecho, mas com leituras
	if s.CruiseS == nil || *s.CruiseS != 0 {
		t.Fatalf("cruzeiro %v", s.CruiseS)
	}
	if s.NReadings != 120 || !hasWarning(r, WarnNoGPS) || !hasWarning(r, WarnNoAbsoluteTime) {
		t.Fatalf("leituras %d avisos %v", s.NReadings, warningCodes(r))
	}
	if r.Series.StepMs != 1000 || r.Series.ExposureStepMs != 1000 {
		t.Fatalf("passo %d/%d", r.Series.StepMs, r.Series.ExposureStepMs)
	}
}

func ptrIf(p *float64, keep bool) *float64 {
	if keep {
		return p
	}
	return nil
}

// Critério 2: as faixas em movimento somam 100% da distância e parado não tem percentual.
func TestSpeedBandsDistribution(t *testing.T) {
	var speeds []float64
	for _, v := range []float64{2.9, 29.9, 60, 110} {
		for i := 0; i < 10; i++ {
			speeds = append(speeds, v)
		}
	}
	p := PartInput{BootID: 1, Readings: readings(
		gen(pidRPM, 0, 39000, 1000, konst(1500)),
		gen(pidSpeed, 0, 39000, 1000, func(t int64) float64 { return speeds[t/1000] }))}
	r := mustAnalyze(t, input(p))
	got := map[string]SpeedBand{}
	var sum float64
	for _, b := range r.SpeedBands {
		got[b.ID] = b
		if b.DistancePct != nil {
			sum += *b.DistancePct
		}
	}
	if len(r.SpeedBands) != 6 || got["parado"].DistancePct != nil || got["parado"].TimeS != 10 || got["ate_30"].TimeS != 10 ||
		got["60_80"].TimeS != 10 || got["acima_110"].TimeS != 10 || got["30_60"].TimeS != 0 {
		t.Fatalf("faixas %+v", r.SpeedBands)
	}
	if math.Abs(sum-100) > 1e-9 || got["acima_110"].ToKmh != nil || got["parado"].ToKmh == nil || *got["parado"].ToKmh != 3 {
		t.Fatalf("soma %v %+v", sum, got)
	}
}

// Critério 3: a faixa do segmento segue a média OBD no intervalo.
func TestRouteBandFollowsOBDMean(t *testing.T) {
	p := PartInput{BootID: 1,
		Readings: readings(
			gen(pidRPM, 0, 10000, 500, konst(1500)),
			gen(pidSpeed, 0, 10000, 500, func(t int64) float64 {
				if (t/500)%2 == 0 {
					return 20
				}
				return 40
			})),
		Positions: []Position{{TMs: 1000, Lat: -25, Lon: -49}, {TMs: 2000, Lat: -25.001, Lon: -49}}}
	r := mustAnalyze(t, input(p))
	seg := r.Series.Route.Segments
	if len(seg.Band) != 1 || seg.Band[0] != "30_60" || seg.SpeedKmh[0] == nil || *seg.SpeedKmh[0] != 30 {
		t.Fatalf("segmentos %+v", seg)
	}
}

// Critério 4: 8 s sem posição é falha de GPS; com gps_falha_s = 10 vira faixa de velocidade.
func TestGPSFailure(t *testing.T) {
	build := func() PartInput {
		return PartInput{BootID: 1, Readings: readings(
			gen(pidRPM, 0, 20000, 1000, konst(1500)), gen(pidSpeed, 0, 20000, 1000, konst(50))),
			Positions: []Position{{TMs: 1000, Lat: -25, Lon: -49}, {TMs: 9000, Lat: -25.01, Lon: -49}, {TMs: 10000, Lat: -25.011, Lon: -49}}}
	}
	r := mustAnalyze(t, input(build()))
	if r.Series.Route.Segments.Band[0] != BandGPSFailure || r.Series.Route.Segments.SpeedKmh[0] != nil || r.Series.Route.Segments.Band[1] != "30_60" {
		t.Fatalf("%+v", r.Series.Route.Segments)
	}
	if *r.Summary.GPSNGaps != 1 || *r.Summary.GPSMaxGapS != 8 {
		t.Fatalf("falhas %d max %v", *r.Summary.GPSNGaps, *r.Summary.GPSMaxGapS)
	}
	in := input(build())
	in.Params.GPSGapS = 10
	r = mustAnalyze(t, in)
	if r.Series.Route.Segments.Band[0] != "30_60" || *r.Summary.GPSNGaps != 0 {
		t.Fatalf("%+v", r.Series.Route.Segments)
	}
}

// Critério 5: sem leitura no segmento usa a última com até 2 s; com mais, sem_velocidade.
func TestSegmentWithoutReading(t *testing.T) {
	p := PartInput{BootID: 1, Readings: readings(
		gen(pidRPM, 0, 30000, 1000, konst(1500)),
		PIDSeries{PID: pidSpeed, Samples: []Sample{{0, 50}, {20000, 60}}}),
		Positions: []Position{{TMs: 1000, Lat: -25, Lon: -49}, {TMs: 2000, Lat: -25.001, Lon: -49},
			{TMs: 3000, Lat: -25.002, Lon: -49}, {TMs: 4000, Lat: -25.003, Lon: -49}}}
	r := mustAnalyze(t, input(p))
	got := r.Series.Route.Segments.Band
	if !reflect.DeepEqual(got, []string{"30_60", "30_60", BandNoSpeed}) {
		t.Fatalf("faixas %v", got)
	}
}

// Rota entre partes: falha_gps (critério 17), e GPS ausente.
func TestNoGPS(t *testing.T) {
	r := mustAnalyze(t, input(simple(1, 0, 20, 50)))
	if !hasWarning(r, WarnNoGPS) || len(r.Series.Route.TMs) != 0 || r.Summary.GPSCoveragePct == nil || *r.Summary.GPSCoveragePct != 0 {
		t.Fatalf("%v %+v", warningCodes(r), r.Series.Route)
	}
	isNil(t, "período", r.Summary.GPSPeriodS)
	isNil(t, "distância gps", r.Summary.GPSDistanceKm)
}

// Critério 7: o mesmo ar admitido rende 0,74 L em gasolina pura e cerca de 0,83 L em E30/E32.
func TestFuelPresets(t *testing.T) {
	build := func(fuel string) Result {
		p := PartInput{BootID: 1, Readings: readings(
			gen(pidRPM, 0, 59000, 1000, konst(1500)), gen(pidSpeed, 0, 59000, 1000, konst(50)),
			gen(pidMAF, 0, 59000, 1000, konst(5)))}
		in := input(p)
		in.Params, _ = in.Params.WithFuel(fuel)
		return mustAnalyze(t, in)
	}
	pura, e30, e32 := build("gasolina_pura"), build("gasolina_e30"), build("gasolina_e32")
	want := *pura.Summary.FuelL * 14.7 * 745 / (13.0 * 750)
	near(t, "e30", e30.Summary.FuelL, want, 1e-9)
	near(t, "e32", e32.Summary.FuelL, want, 1e-9)
	near(t, "ar", pura.Summary.IntakeAirG, 5*60, 1e-9)
	near(t, "ar e30", e30.Summary.IntakeAirG, 5*60, 1e-9)
	near(t, "cobertura", e30.Summary.FuelCoveragePct, 100, 1e-9)
	if hasWarning(e30, WarnLowMAFCoverage) {
		t.Fatal("cobertura é total")
	}
	// km/L = distância x AFR x densidade / ar, como o ranking recalcula
	km := *e30.Summary.DistanceKm * 13.0 * 750 / *e30.Summary.IntakeAirG
	near(t, "km/L", e30.Summary.KmPerL, km, 1e-9)
	// g/s de ar -> L/h: 5 / 13 / 750 x 3600
	near(t, "L/h", r0(e30.Series.Series.FuelLH), 5.0/13/750*3600, 1e-12)
}

func r0(s []*float64) *float64 { return s[0] }

func TestFuelPresetErrors(t *testing.T) {
	if _, err := DefaultParams().WithFuel("lava"); !errors.Is(err, ErrInvalidParams) {
		t.Fatalf("%v", err)
	}
	custom, err := DefaultParams().WithFuel(FuelCustom)
	if err != nil || custom.Validate() == nil || !errors.Is(custom.Validate(), ErrInvalidParams) {
		t.Fatalf("personalizado sem afr deve ser inválido: %v %v", err, custom.Validate())
	}
	custom.AFR, custom.DensityGL = 10, 800
	if err := custom.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, f := range FuelPresets {
		p, err := DefaultParams().WithFuel(f.ID)
		if err != nil || p.AFR != f.AFR || p.DensityGL != f.DensityGL || p.Validate() != nil {
			t.Fatalf("%s: %+v %v", f.ID, p, err)
		}
	}
	bad := DefaultParams()
	bad.SeriesMaxPoints = 1
	if _, err := Analyze(Input{Entry: bootEntry(), Params: bad}); !errors.Is(err, ErrInvalidParams) {
		t.Fatalf("%v", err)
	}
}

// Critério 9: o pico de uma frenagem forte é exposto como calculado; critério 10: taxa nula em viagem curta.
func TestHardBrakePeakUnfiltered(t *testing.T) {
	// 100 km/h até 10 s, queda de 37,8 km/h/s (-10,5 m/s²) por 2 s, depois constante
	speed := func(t int64) float64 {
		switch {
		case t < 10000:
			return 100
		case t < 12000:
			return 100 - 37.8*float64(t-10000)/1000
		}
		return 100 - 37.8*2
	}
	p := PartInput{BootID: 1, Readings: readings(
		gen(pidRPM, 0, 30000, 100, konst(2000)), gen(pidSpeed, 0, 30000, 100, speed))}
	r := mustAnalyze(t, input(p))
	var brakes []Event
	for _, e := range r.Events {
		if e.Type == EventHardBrake {
			brakes = append(brakes, e)
		}
	}
	if len(brakes) != 1 || brakes[0].PeakUnit != "m/s2" {
		t.Fatalf("eventos %+v", r.Events)
	}
	if math.Abs(brakes[0].Peak+10.5) > 1e-6 {
		t.Fatalf("pico %v", brakes[0].Peak)
	}
	if brakes[0].StartSpeedKmh == nil || brakes[0].Lat != nil {
		t.Fatalf("%+v", brakes[0])
	}
	if *r.Summary.NHardBrake != 1 || *r.Summary.NHardAccel != 0 {
		t.Fatalf("contagem %d/%d", *r.Summary.NHardBrake, *r.Summary.NHardAccel)
	}
	isNil(t, "freadas/100km", r.Summary.HardBrakePer100Km) // distância < 20 km
	isNil(t, "acel/100km", r.Summary.HardAccelPer100Km)
}

// Critério 10: com 25 km as taxas vêm preenchidas.
func TestRatesWithLongDistance(t *testing.T) {
	// 90 km/h por 1000 s = 25 km, com uma frenagem brusca
	speed := func(t int64) float64 {
		if t >= 100000 && t < 102000 {
			return 90 - 36*float64(t-100000)/1000 // -10 m/s²
		}
		if t >= 102000 && t < 104000 {
			return 90 - 72 + 36*float64(t-102000)/1000
		}
		return 90
	}
	p := PartInput{BootID: 1, Readings: readings(
		gen(pidRPM, 0, 999000, 1000, konst(2500)), gen(pidSpeed, 0, 999000, 1000, speed))}
	r := mustAnalyze(t, input(p))
	if *r.Summary.DistanceKm < 20 || r.Summary.HardBrakePer100Km == nil || *r.Summary.NHardBrake < 1 {
		t.Fatalf("%v %+v", *r.Summary.DistanceKm, r.Summary.HardBrakePer100Km)
	}
	near(t, "taxa", r.Summary.HardBrakePer100Km, float64(*r.Summary.NHardBrake)*100 / *r.Summary.DistanceKm, 1e-9)
}

// Critério 11: 23% do tempo em pedal alto dispara o aviso; 5% não.
func TestPedalHighWarning(t *testing.T) {
	build := func(highSeconds int64) Result {
		p := PartInput{BootID: 1, Readings: readings(
			gen(pidRPM, 0, 99000, 1000, konst(2500)), gen(pidSpeed, 0, 99000, 1000, konst(60)),
			gen(pidPedal, 0, 99000, 1000, func(t int64) float64 {
				if t/1000 < highSeconds {
					return 90
				}
				return 30
			}))}
		return mustAnalyze(t, input(p))
	}
	r := build(23)
	if !hasWarning(r, WarnFuelMayBeUnderrated) {
		t.Fatalf("avisos %v", warningCodes(r))
	}
	near(t, "pedal alto", r.Summary.PedalHighS, 23, 1e-9)
	if hasWarning(build(5), WarnFuelMayBeUnderrated) {
		t.Fatal("5% não deve avisar")
	}
}

// Critério 13: reprodutibilidade.
func TestDeterminism(t *testing.T) {
	in := input(gearPart(1, 0), gearPart(2, 200000))
	in.Entry = Entry{Type: EntryInterval, DongleID: 1, T0: &t0, T1: ptrTime(t0.Add(time.Hour))}
	in.Params.IdleRPM = f64(600)
	a, b := mustAnalyze(t, in), mustAnalyze(t, in)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("resultados diferentes para a mesma entrada")
	}
	if len(a.Gears) == 0 {
		t.Fatal("o teste deveria exercitar as marchas")
	}
}

func ptrTime(t time.Time) *time.Time { return &t }

// Critério 14: Entry.Validate.
func TestEntryValidate(t *testing.T) {
	t1 := t0.Add(time.Hour)
	ok := []Entry{bootEntry(), {Type: EntryInterval, DongleID: 1, T0: &t0, T1: &t1}}
	for _, e := range ok {
		if err := e.Validate(); err != nil {
			t.Errorf("%+v: %v", e, err)
		}
	}
	bad := []Entry{
		{},                // nenhuma
		{Type: EntryBoot}, // boot sem boot_id
		{Type: EntryBoot, BootID: i32p(1), T0: &t0, T1: &t1},     // as duas
		{Type: EntryInterval, T0: &t0, T1: &t1, BootID: i32p(1)}, // as duas
		{Type: EntryInterval, T0: &t0},                           // sem t1
		{Type: EntryInterval, T0: &t1, T1: &t0},                  // t1 antes de t0
		{Type: EntryInterval, T0: &t0, T1: &t0},                  // t1 = t0
		{Type: "viagem", BootID: i32p(1)},                        // tipo desconhecido
	}
	for _, e := range bad {
		if err := e.Validate(); !errors.Is(err, ErrInvalidEntry) {
			t.Errorf("%+v deveria ser inválida: %v", e, err)
		}
		if _, err := Analyze(Input{Entry: e, Params: DefaultParams()}); !errors.Is(err, ErrInvalidEntry) {
			t.Errorf("Analyze aceitou %+v", e)
		}
	}
}

// Critério 20: aceleração RMS.
func TestAccelRMS(t *testing.T) {
	build := func(speeds []float64) Result {
		p := PartInput{BootID: 1, Readings: readings(
			gen(pidRPM, 0, int64(len(speeds)-1)*1000, 1000, konst(1500)),
			gen(pidSpeed, 0, int64(len(speeds)-1)*1000, 1000, func(t int64) float64 { return speeds[t/1000] }))}
		return mustAnalyze(t, input(p))
	}
	// a[i] = (v[i+1]-v[i-1])/7,2 com passo de 1 s: 0, 0, +3, -3
	r := build([]float64{60, 60, 60, 60, 81.6, 38.4})
	near(t, "rms", r.Summary.AccelRMSMS2, math.Sqrt((0+0+9+9)/4.0), 1e-9)
	// rampa de 3,6 km/h/s = 1 m/s²
	ramp := make([]float64, 20)
	for i := range ramp {
		ramp[i] = 40 + 3.6*float64(i)
	}
	near(t, "rampa", build(ramp).Summary.AccelRMSMS2, 1, 1e-9)
	// parado não entra
	isNil(t, "parado", build([]float64{0, 0, 0, 0, 0, 0}).Summary.AccelRMSMS2)
}

// Critérios 21 e 22: cruzeiro.
func TestCruise(t *testing.T) {
	build := func(n int64, speed func(int64) float64) Result {
		p := PartInput{BootID: 1, Readings: readings(
			gen(pidRPM, 0, (n-1)*1000, 1000, konst(2000)), gen(pidSpeed, 0, (n-1)*1000, 1000, speed))}
		return mustAnalyze(t, input(p))
	}
	wobble := func(t int64) float64 { return 80 + float64((t/1000)%7-3) } // 80 +- 3
	near(t, "60 s", build(60, wobble).Summary.CruiseS, 60, 1e-9)
	near(t, "25 s", build(25, wobble).Summary.CruiseS, 0, 1e-9)
	near(t, "30 km/h", build(60, konst(30)).Summary.CruiseS, 0, 1e-9)
	// amplitude acima de 10 km/h quebra o trecho
	near(t, "rampa", build(60, func(t int64) float64 { return 45 + float64(t/1000)*2 }).Summary.CruiseS, 0, 1e-9)
	// sem leituras de velocidade, o tempo em cruzeiro é nulo
	p := PartInput{BootID: 1, Readings: readings(gen(pidRPM, 0, 59000, 1000, konst(2000)))}
	r := mustAnalyze(t, input(p))
	isNil(t, "sem velocidade", r.Summary.CruiseS)
	isNil(t, "distância sem velocidade", r.Summary.DistanceKm)
	isNil(t, "movimento sem velocidade", r.Summary.MovingS)
	if len(r.SpeedBands) != 0 {
		t.Fatalf("faixas %+v", r.SpeedBands)
	}
}

// Critério 23: 20 s no fim de uma parte e 20 s no início da seguinte não formam um trecho.
func TestCruiseDoesNotCrossParts(t *testing.T) {
	a := PartInput{BootID: 1, Readings: readings(gen(pidRPM, 0, 19000, 1000, konst(2000)), gen(pidSpeed, 0, 19000, 1000, konst(80)))}
	b := PartInput{BootID: 2, Readings: readings(gen(pidRPM, 20000, 39000, 1000, konst(2000)), gen(pidSpeed, 20000, 39000, 1000, konst(80)))}
	in := input(a, b)
	in.Entry = Entry{Type: EntryInterval, DongleID: 1, T0: &t0, T1: ptrTime(t0.Add(time.Hour))}
	r := mustAnalyze(t, in)
	near(t, "cruzeiro", r.Summary.CruiseS, 0, 1e-9)
	near(t, "movimento", r.Summary.MovingS, 40, 1e-9)
}

// Critérios 15 a 17 e 19: intervalo com duas partes.
func TestIntervalTwoParts(t *testing.T) {
	a := gearPart(1, 0)
	b := gearPart(2, 200000)
	a.Positions = []Position{{TMs: 5000, Lat: -25, Lon: -49}, {TMs: 6000, Lat: -25.001, Lon: -49}}
	b.Positions = []Position{{TMs: 205000, Lat: -25.1, Lon: -49}}
	a.Clock = &Clock{AMs: 1, UncertaintyMs: 100}
	b.Clock = &Clock{AMs: 2, UncertaintyMs: 240}
	in := input(b, a) // fora de ordem de propósito
	in.Params.IdleRPM = f64(600)
	in.Entry = Entry{Type: EntryInterval, DongleID: 1, T0: &t0, T1: ptrTime(t0.Add(time.Hour))}
	r := mustAnalyze(t, in)
	if len(r.Parts) != 2 || r.Parts[0].BootID != 1 || r.Parts[1].BootID != 2 {
		t.Fatalf("partes %+v", r.Parts)
	}
	s := r.Summary
	partDur := float64(*r.Parts[0].EndTMs-*r.Parts[0].StartTMs) / 1000
	near(t, "duração", s.DurationS, 2*partDur, 1e-9)
	near(t, "sem dados", s.NoDataS, float64(200000)/1000-partDur, 1e-9)
	// a rota não liga as partes: segmento entre elas é falha_gps
	if got := r.Series.Route.Segments.Band; len(got) != 2 || got[1] != BandGPSFailure {
		t.Fatalf("rota %v", got)
	}
	// critério 19: a mesma marcha tem o mesmo número nas duas partes
	byPart := map[int]map[int]bool{}
	for k, tm := range r.Series.Series.TMs {
		g := r.Series.Series.Gear[k]
		if g == nil || *g == 0 {
			continue
		}
		part := 0
		if tm >= 200000 {
			part = 1
		}
		if byPart[part] == nil {
			byPart[part] = map[int]bool{}
		}
		byPart[part][*g] = true
	}
	if !reflect.DeepEqual(byPart[0], byPart[1]) || len(byPart[0]) != 3 {
		t.Fatalf("marchas por parte %v", byPart)
	}
	// trocas não cruzam partes: 2 por parte
	if len(r.GearShifts) != 4 {
		t.Fatalf("trocas %d", len(r.GearShifts))
	}
	if hasWarning(r, WarnOverlappingParts) {
		t.Fatalf("avisos %v", warningCodes(r))
	}
	// relógio do intervalo: t0 em ms e a maior incerteza
	if *s.ClockOffsetMs != t0.UnixMilli() || *s.ClockUncertaintyMs != 240 {
		t.Fatalf("relógio %v %v", *s.ClockOffsetMs, *s.ClockUncertaintyMs)
	}
	if !s.StartedAt.Equal(t0.Add(time.Duration(*s.StartTMs) * time.Millisecond)) {
		t.Fatalf("início %v", s.StartedAt)
	}
	// separador nulo logo após cada parte
	nulls := 0
	for _, sp := range r.Series.Series.Speed {
		if sp == nil {
			nulls++
		}
	}
	if nulls != 2 {
		t.Fatalf("separadores %d", nulls)
	}
}

// Partes que se cruzam: a anterior termina no início da seguinte e há aviso.
func TestOverlappingParts(t *testing.T) {
	a := simple(1, 0, 60, 50)
	b := simple(2, 40000, 60, 50)
	in := input(a, b)
	in.Entry = Entry{Type: EntryInterval, DongleID: 1, T0: &t0, T1: ptrTime(t0.Add(time.Hour))}
	r := mustAnalyze(t, in)
	if !hasWarning(r, WarnOverlappingParts) {
		t.Fatalf("avisos %v", warningCodes(r))
	}
	if r.Parts[0].DataEndTMs != 39000 || r.Parts[1].DataStartTMs != 40000 {
		t.Fatalf("partes %+v", r.Parts)
	}
	// incerteza que estende além do intervalo entre elas também avisa, sem cortar
	c := simple(1, 0, 10, 50)
	c.Clock = &Clock{UncertaintyMs: 3000}
	d := simple(2, 12000, 10, 50)
	d.Clock = &Clock{UncertaintyMs: 3000}
	in = input(c, d)
	in.Entry = Entry{Type: EntryInterval, DongleID: 1, T0: &t0, T1: ptrTime(t0.Add(time.Hour))}
	r = mustAnalyze(t, in)
	if !hasWarning(r, WarnOverlappingParts) || r.Parts[0].DataEndTMs != 9000 {
		t.Fatalf("%v %+v", warningCodes(r), r.Parts)
	}
}

// Uma parte sem motor ligado continua listada, com janela nula; a outra define a viagem.
func TestPartWithoutEngine(t *testing.T) {
	off := PartInput{BootID: 1, Readings: readings(gen(pidRPM, 0, 9000, 1000, konst(0)), gen(pidSpeed, 0, 9000, 1000, konst(0)))}
	on := simple(2, 20000, 30, 50)
	in := input(off, on)
	in.Entry = Entry{Type: EntryInterval, DongleID: 1, T0: &t0, T1: ptrTime(t0.Add(time.Hour))}
	r := mustAnalyze(t, in)
	if len(r.Parts) != 2 || r.Parts[0].StartTMs != nil || r.Parts[1].StartTMs == nil || *r.Summary.StartTMs != 20000 {
		t.Fatalf("%+v", r.Parts)
	}
	if hasWarning(r, WarnEngineNeverOn) {
		t.Fatal("há motor ligado em uma parte")
	}
}

func TestEngineNeverOn(t *testing.T) {
	p := PartInput{BootID: 1, Readings: readings(gen(pidRPM, 0, 19000, 1000, konst(0)), gen(pidSpeed, 0, 19000, 1000, konst(0)))}
	r := mustAnalyze(t, input(p))
	if !hasWarning(r, WarnEngineNeverOn) || r.Summary.StartTMs == nil || *r.Summary.StartTMs != 0 {
		t.Fatalf("%v", warningCodes(r))
	}
	near(t, "motor", r.Summary.EngineOnS, 0, 1e-9)
	if r.Parts[0].StartTMs != nil {
		t.Fatal("a parte não teve motor ligado")
	}
}

// Sem leituras: valores nulos e o aviso; parcial e avisos do carregador entram.
func TestEmptyTrip(t *testing.T) {
	in := input()
	in.Partial = true
	in.EntryWarnings = []Warning{{Code: WarnBootNoTime, Detail: map[string]any{"boot_id": 3}}}
	r := mustAnalyze(t, in)
	if !reflect.DeepEqual(warningCodes(r), []string{WarnBootNoTime, WarnNoReadings, WarnPartialData}) {
		t.Fatalf("avisos %v", warningCodes(r))
	}
	if r.Summary.DistanceKm != nil || r.Summary.StartTMs != nil || r.Series.Series.TMs == nil || r.Parts == nil || r.SpeedBands == nil {
		t.Fatalf("%+v", r.Summary)
	}
	// parte sem nenhuma leitura também é viagem vazia
	r = mustAnalyze(t, input(PartInput{BootID: 1, Readings: readings(PIDSeries{PID: pidRPM})}))
	if !hasWarning(r, WarnNoReadings) {
		t.Fatalf("%v", warningCodes(r))
	}
	// intervalo vazio guarda t0 como deslocamento
	in = input()
	in.Entry = Entry{Type: EntryInterval, DongleID: 1, T0: &t0, T1: ptrTime(t0.Add(time.Hour))}
	r = mustAnalyze(t, in)
	if r.Summary.ClockOffsetMs == nil || *r.Summary.ClockOffsetMs != t0.UnixMilli() {
		t.Fatalf("%+v", r.Summary.ClockOffsetMs)
	}
}

// Entrada boot: relógio, hora absoluta e sem_hora_absoluta quando não há conversão.
func TestBootClock(t *testing.T) {
	p := simple(1, 0, 20, 50)
	p.Clock = &Clock{AMs: 1_793_206_969_000, B: 50e-6, UncertaintyMs: 180}
	r := mustAnalyze(t, input(p))
	s := r.Summary
	if *s.ClockOffsetMs != 1_793_206_969_000 || *s.ClockUncertaintyMs != 180 || hasWarning(r, WarnNoAbsoluteTime) {
		t.Fatalf("%+v %v", s, warningCodes(r))
	}
	want := time.UnixMilli(int64(math.Round(1_793_206_969_000 + (1+50e-6)*19000))).UTC()
	if !s.EndedAt.Equal(want) || !s.StartedAt.Equal(time.UnixMilli(1_793_206_969_000)) {
		t.Fatalf("%v %v", s.StartedAt, s.EndedAt)
	}
	p.Clock = nil
	r = mustAnalyze(t, input(p))
	if !hasWarning(r, WarnNoAbsoluteTime) || r.Summary.StartedAt != nil || r.Summary.ClockOffsetMs != nil {
		t.Fatalf("%v", warningCodes(r))
	}
}

// Critério 8 e 3 da API1: os parâmetros mudam a classificação e voltam no resultado.
func TestParamsChangeResult(t *testing.T) {
	// 20 km/h a 800 rpm, depois parado a 700 rpm por 30 s
	p := PartInput{BootID: 1, Readings: readings(
		gen(pidRPM, 0, 59000, 1000, func(t int64) float64 {
			if t < 30000 {
				return 800
			}
			return 700
		}),
		gen(pidSpeed, 0, 59000, 1000, func(t int64) float64 {
			if t < 30000 {
				return 20
			}
			return 0
		}))}
	base := mustAnalyze(t, input(p))
	if *base.Summary.IdleRPMOrigin != "estimada" || *base.Summary.IdleRPM != 700 || hasWarning(base, WarnIdleFallback) {
		t.Fatalf("%v %v", *base.Summary.IdleRPM, *base.Summary.IdleRPMOrigin)
	}
	// 800 <= 700 + 150: ponto morto/embreagem, não vira marcha nem troca
	near(t, "ponto morto", base.Summary.NeutralClutchS, 30, 1e-9)
	if len(base.Gears) != 0 || len(base.GearShifts) != 0 {
		t.Fatalf("marchas %+v", base.Gears)
	}
	in := input(p)
	in.Params.IdleMarginRPM = 50
	alt := mustAnalyze(t, in)
	near(t, "ponto morto margem 50", alt.Summary.NeutralClutchS, 0, 1e-9)
	near(t, "margem", alt.Summary.IdleMarginRPM, 50, 0)
	in = input(p)
	in.Params.IdleRPM = f64(500)
	alt = mustAnalyze(t, in)
	if *alt.Summary.IdleRPMOrigin != "informada" || *alt.Summary.IdleRPM != 500 {
		t.Fatalf("%+v", alt.Summary.IdleRPM)
	}
	near(t, "ponto morto com 500", alt.Summary.NeutralClutchS, 0, 1e-9)
	// gps_falha_s volta em parametros
	in.Params.GPSGapS = 12
	if alt = mustAnalyze(t, in); alt.Params.GPSGapS != 12 || alt.Params.IdleRPM == nil {
		t.Fatalf("%+v", alt.Params)
	}
	// poucas amostras parado: marcha lenta de reserva e aviso
	few := PartInput{BootID: 1, Readings: readings(gen(pidRPM, 0, 19000, 1000, konst(900)), gen(pidSpeed, 0, 19000, 1000, konst(50)))}
	r := mustAnalyze(t, input(few))
	if *r.Summary.IdleRPMOrigin != "reserva" || *r.Summary.IdleRPM != 750 || !hasWarning(r, WarnIdleFallback) {
		t.Fatalf("%v", warningCodes(r))
	}
}

// Marcha lenta longa, paradas e posições.
func TestStopsAndLongIdle(t *testing.T) {
	// 10 s a 50 km/h, 70 s parado com motor ligado, 10 s a 50 km/h; GPS perto da parada
	speed := func(t int64) float64 {
		if t >= 10000 && t < 80000 {
			return 0
		}
		return 50
	}
	p := PartInput{BootID: 1, Readings: readings(gen(pidRPM, 0, 89000, 1000, konst(800)), gen(pidSpeed, 0, 89000, 1000, speed)),
		Positions: []Position{{TMs: 9000, Lat: -25.1, Lon: -49.1}, {TMs: 30000, Lat: -25.2, Lon: -49.2}}}
	r := mustAnalyze(t, input(p))
	if len(r.Stops) != 1 || r.Stops[0].StartTMs != 10000 || r.Stops[0].DurationS != 70 || !r.Stops[0].EngineOn {
		t.Fatalf("paradas %+v", r.Stops)
	}
	if r.Stops[0].Lat == nil || *r.Stops[0].Lat != -25.1 { // posição mais próxima do início, a 1 s
		t.Fatalf("posição %v", r.Stops[0].Lat)
	}
	var idle *Event
	for i, e := range r.Events {
		if e.Type == EventLongIdle {
			idle = &r.Events[i]
		}
	}
	if idle == nil || idle.PeakUnit != "s" || idle.Peak != 70 || idle.DurationS != 70 {
		t.Fatalf("eventos %+v", r.Events)
	}
	near(t, "marcha lenta", r.Summary.IdleS, 70, 1e-9)
	// parada curta (3 s) não vira marcador
	short := PartInput{BootID: 1, Readings: readings(gen(pidRPM, 0, 29000, 1000, konst(800)),
		gen(pidSpeed, 0, 29000, 1000, func(t int64) float64 {
			if t >= 10000 && t < 13000 {
				return 0
			}
			return 50
		}))}
	if r = mustAnalyze(t, input(short)); len(r.Stops) != 0 {
		t.Fatalf("%+v", r.Stops)
	}
	// posição distante (mais de gps_falha_s) fica nula
	far := p
	far.Positions = []Position{{TMs: 40000, Lat: -25.3, Lon: -49.3}, {TMs: 41000, Lat: -25.3, Lon: -49.3}}
	if r = mustAnalyze(t, input(far)); r.Stops[0].Lat != nil {
		t.Fatalf("%v", r.Stops[0].Lat)
	}
}

// Distribuições de rotação e pedal, matriz e indicadores do condutor.
func TestDistributionsAndDriver(t *testing.T) {
	p := PartInput{BootID: 1, Readings: readings(
		gen(pidRPM, 0, 99000, 1000, func(t int64) float64 {
			if t < 50000 {
				return 800
			}
			return 2200
		}),
		gen(pidSpeed, 0, 99000, 1000, konst(40)),
		gen(pidPedal, 0, 99000, 1000, func(t int64) float64 {
			if t < 50000 {
				return 2
			}
			return 50
		}),
		gen(pidCoolant, 0, 99000, 1000, func(t int64) float64 { return 50 + float64(t/1000) }),
	)}
	r := mustAnalyze(t, input(p))
	if len(r.Series.RPMDistribution) != 9 || r.Series.RPMDistribution[0].TimeS != 50 || r.Series.RPMDistribution[4].TimeS != 0 ||
		r.Series.RPMDistribution[3].TimeS != 50 || r.Series.RPMDistribution[8].To != nil {
		t.Fatalf("rotação %+v", r.Series.RPMDistribution)
	}
	if math.Abs(r.Series.RPMDistribution[0].Pct-50) > 1e-9 {
		t.Fatalf("pct %v", r.Series.RPMDistribution[0].Pct)
	}
	ped := r.Series.PedalDistribution
	if len(ped) != 6 || ped[0].TimeS != 50 || ped[0].Label == nil || *ped[0].Label != "Solto" || ped[3].TimeS != 50 || ped[5].To != 100 {
		t.Fatalf("pedal %+v", ped)
	}
	m := r.Series.RPMPedal
	if m.RPMBin != 500 || m.PedalBin != 10 || len(m.TimeS) != 5 || m.TimeS[1][0] != 50 || m.TimeS[4][5] != 50 {
		t.Fatalf("matriz %+v", m)
	}
	near(t, "pedal médio", r.Summary.PedalMeanPct, 26, 1e-9)
	// freio-motor: pedal <= 3, rpm >= 1200: a 1ª metade tem rpm 800, então 0%
	near(t, "freio-motor", r.Summary.EngineBrakePct, 0, 1e-9)
	// aquecimento: primeira leitura >= 80 °C em t = 30 s; temperatura máxima 149
	near(t, "aquecimento", r.Summary.WarmupS, 30, 1e-9)
	near(t, "máx", r.Summary.MaxCoolantC, 149, 1e-9)
	// esforço com motor frio: < 60 °C (t < 10 s) e pedal <= 60 e rpm 800: nenhum
	near(t, "frio", r.Summary.ColdLoadS, 0, 1e-9)
	// variação do pedal: salto de 48 pontos em 1 s uma vez; p95 de 99 taxas fica entre 0 e 48
	if r.Summary.PedalRateP95PctS == nil || *r.Summary.PedalRateP95PctS < 0 || *r.Summary.PedalRateP95PctS > 48 {
		t.Fatalf("p95 %v", r.Summary.PedalRateP95PctS)
	}
}

func TestEngineBrakeAndColdLoad(t *testing.T) {
	p := PartInput{BootID: 1, Readings: readings(
		gen(pidRPM, 0, 59000, 1000, konst(3500)), gen(pidSpeed, 0, 59000, 1000, konst(60)),
		gen(pidPedal, 0, 59000, 1000, func(t int64) float64 {
			if t < 20000 {
				return 0
			}
			return 20
		}),
		gen(pidCoolant, 0, 59000, 1000, konst(40)))}
	r := mustAnalyze(t, input(p))
	near(t, "freio-motor", r.Summary.EngineBrakePct, 20.0/60*100, 1e-9)
	near(t, "frio", r.Summary.ColdLoadS, 60, 1e-9) // rpm 3500 > 3000 com motor frio o tempo todo
	isNil(t, "aquecimento", r.Summary.WarmupS)
}

// Séries expostas: reduzidas a serie_max_pontos com o passo informado.
func TestSeriesReduction(t *testing.T) {
	r := mustAnalyze(t, input(simple(1, 0, 12000, 50)))
	if r.Series.ExposureStepMs != 3000 || r.Series.StepMs != 1000 {
		t.Fatalf("passo %d/%d", r.Series.StepMs, r.Series.ExposureStepMs)
	}
	s := r.Series.Series
	if len(s.TMs) != 4001 || s.Speed[len(s.Speed)-1] != nil || s.TMs[1] != 3000 {
		t.Fatalf("pontos %d, t[1]=%d", len(s.TMs), s.TMs[1])
	}
	// os cálculos usam a grade completa
	near(t, "distância", r.Summary.DistanceKm, 50.0*12000/3600, 1e-9)
	last := s.DistCumKm[len(s.DistCumKm)-2]
	if *last > *r.Summary.DistanceKm || *last < *r.Summary.DistanceKm-0.5 {
		t.Fatalf("acumulado %v", *last)
	}
}

// Séries lentas: tanque com mediana móvel de ±3 leituras, separador entre partes.
func TestSlowSeries(t *testing.T) {
	p := PartInput{BootID: 1, Readings: readings(gen(pidRPM, 0, 19000, 1000, konst(1500)),
		gen(pidTank, 0, 19000, 1000, func(t int64) float64 {
			if t == 10000 {
				return 0 // pico espúrio que a mediana corta
			}
			return 80
		}),
		gen(pidCoolant, 0, 19000, 5000, konst(70)))}
	r := mustAnalyze(t, input(p))
	tk := r.Series.Slow.Tank
	if *tk.Pct[10] != 0 || *tk.MedianPct[10] != 80 || tk.Pct[len(tk.Pct)-1] != nil {
		t.Fatalf("tanque %v", tk.MedianPct)
	}
	if len(r.Series.Slow.Temperature.TMs) != 5 || r.Series.Slow.Temperature.C[4] != nil {
		t.Fatalf("temperatura %+v", r.Series.Slow.Temperature)
	}
}

// Qualidade: períodos medianos por PID e taxa de leituras.
func TestQuality(t *testing.T) {
	p := PartInput{BootID: 1, Readings: readings(gen(pidRPM, 0, 9900, 100, konst(1500)), gen(pidSpeed, 0, 9900, 100, konst(50)),
		gen(pidCoolant, 0, 9000, 3000, konst(70)))}
	r := mustAnalyze(t, input(p))
	if r.Summary.PIDPeriodsMs["0x0C"] != 100 || r.Summary.PIDPeriodsMs["0x05"] != 3000 || r.Series.StepMs != 100 {
		t.Fatalf("%+v passo %d", r.Summary.PIDPeriodsMs, r.Series.StepMs)
	}
	near(t, "taxa", r.Summary.ReadingsPerS, float64(r.Summary.NReadings)/9.9, 1e-9)
}

// Passo da grade: múltiplo de 50 entre 100 e 1000; sem PIDs rápidos, 1000.
func TestGridStep(t *testing.T) {
	step := func(period int64) int32 {
		p := PartInput{BootID: 1, Readings: readings(gen(pidRPM, 0, 60000, period, konst(1500)), gen(pidSpeed, 0, 60000, period, konst(50)))}
		return mustAnalyze(t, input(p)).Series.StepMs
	}
	for period, want := range map[int64]int32{20: 100, 130: 150, 160: 150, 175: 200, 5000: 1000} {
		if got := step(period); got != want {
			t.Errorf("período %d: passo %d, esperado %d", period, got, want)
		}
	}
	p := PartInput{BootID: 1, Readings: readings(gen(pidCoolant, 0, 60000, 5000, konst(70)), gen(pidTank, 0, 60000, 5000, konst(70)))}
	if got := mustAnalyze(t, input(p)).Series.StepMs; got != 1000 {
		t.Errorf("sem PIDs rápidos: %d", got)
	}
}

// Valores inválidos e repetidos são descartados; a primeira leitura de cada t_ms vale.
func TestCleanSeries(t *testing.T) {
	s := cleanSeries([]Sample{{2000, 2}, {1000, 1}, {1000, 99}, {3000, math.NaN()}, {4000, math.Inf(1)}, {5000, 5}})
	if !reflect.DeepEqual(s.t, []int64{1000, 2000, 5000}) || !reflect.DeepEqual(s.v, []float64{1, 2, 5}) {
		t.Fatalf("%+v", s)
	}
}

func TestQuantile(t *testing.T) {
	if !math.IsNaN(median(nil)) {
		t.Fatal("mediana vazia")
	}
	a := []float64{4, 1, 3, 2}
	if median(a) != 2.5 || quantile(a, 0.25) != 1.75 || quantile(a, 1) != 4 || quantile(a, 0) != 1 {
		t.Fatalf("%v %v", median(a), quantile(a, 0.25))
	}
	if d := haversineKm(0, 0, 0, 1); math.Abs(d-111.19) > 0.01 {
		t.Fatalf("haversine %v", d)
	}
}
