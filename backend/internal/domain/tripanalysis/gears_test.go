package tripanalysis

import (
	"math"
	"testing"
)

// gearPart é uma viagem sintética com três patamares de rpm/(km/h) (100, 60 e 40) em rampas de velocidade, a cada 100 ms,
// começando em from. Marcha lenta informada em 600 rpm nos testes.
func gearPart(boot int32, from int64) PartInput { return buildGearPart(boot, from, false) }

// buildGearPart com reverse=true percorre os patamares ao contrário (3ª, 2ª, 1ª): só reduções.
func buildGearPart(boot int32, from int64, reverse bool) PartInput {
	type plateau struct {
		start, end int64
		v0, v1, k  float64
	}
	plateaus := []plateau{{0, 10000, 10, 20, 100}, {10000, 25000, 22, 45, 60}, {25000, 45000, 45, 75, 40}}
	at := func(t int64) (v, rpm float64) {
		if reverse {
			t = 44999 - t
		}
		for _, p := range plateaus {
			if t >= p.start && t < p.end {
				v = p.v0 + (p.v1-p.v0)*float64(t-p.start)/float64(p.end-p.start)
				return v, v * p.k
			}
		}
		return 75, 3000
	}
	return PartInput{BootID: boot, Readings: readings(
		gen(pidRPM, from, from+45000, 100, func(t int64) float64 { _, r := at(t - from); return r }),
		gen(pidSpeed, from, from+45000, 100, func(t int64) float64 { v, _ := at(t - from); return v }),
		gen(pidPedal, from, from+45000, 100, konst(30)))}
}

func TestGearDetection(t *testing.T) {
	in := input(gearPart(1, 0))
	in.Params.IdleRPM = f64(600)
	r := mustAnalyze(t, in)
	if len(r.Gears) != 3 {
		t.Fatalf("patamares %+v", r.Gears)
	}
	for i, want := range []float64{100, 60, 40} {
		if g := r.Gears[i]; g.Gear != i+1 || math.Abs(g.RPMPerKmh-want)/want > 0.03 || g.TimeS <= 0 {
			t.Fatalf("marcha %d: %+v (esperado ~%v)", i+1, g, want)
		}
	}
	var pct float64
	for _, g := range r.Gears {
		pct += g.TimePct
	}
	if math.Abs(pct-100) > 1e-9 {
		t.Fatalf("soma %v", pct)
	}
	if len(r.GearShifts) != 2 || r.GearShifts[0].Direction != ShiftUp || r.GearShifts[0].From != 1 || r.GearShifts[0].To != 2 ||
		r.GearShifts[1].To != 3 {
		t.Fatalf("trocas %+v", r.GearShifts)
	}
	sh := r.GearShifts[0]
	if sh.TMs < 9000 || sh.TMs > 10000 || sh.RPMBefore < 1900 || sh.RPMAfter > 1400 || sh.SpeedKmh < 19 {
		t.Fatalf("troca %+v", sh)
	}
	s := r.Summary
	near(t, "mediana subida", s.UpshiftRPMMedian, (sh.RPMBefore+r.GearShifts[1].RPMBefore)/2, 1e-9)
	if *s.UpshiftN != 2 || *s.DownshiftN != 0 || s.DownshiftRPMMedian != nil {
		t.Fatalf("indicadores %d %d", *s.UpshiftN, *s.DownshiftN)
	}
	if s.UpshiftRPMP25 == nil || s.UpshiftRPMP75 == nil || *s.UpshiftRPMP25 > *s.UpshiftRPMP75 {
		t.Fatalf("quartis %v %v", s.UpshiftRPMP25, s.UpshiftRPMP75)
	}
}

// Reduções: o percurso inverso gera trocas de redução.
func TestGearDownshifts(t *testing.T) {
	in := input(buildGearPart(1, 0, true))
	in.Params.IdleRPM = f64(600)
	r := mustAnalyze(t, in)
	if len(r.Gears) != 3 || len(r.GearShifts) != 2 {
		t.Fatalf("patamares %d trocas %+v", len(r.Gears), r.GearShifts)
	}
	for _, sh := range r.GearShifts {
		if sh.Direction != ShiftDown || sh.To >= sh.From {
			t.Fatalf("troca %+v", sh)
		}
	}
	s := r.Summary
	if *s.DownshiftN != 2 || *s.UpshiftN != 0 || s.UpshiftRPMMedian != nil || s.DownshiftRPMMedian == nil {
		t.Fatalf("indicadores %+v", s)
	}
}

// Poucas amostras estáveis: não há marchas, e o ponto morto continua contado.
func TestNoGearsWithFewSamples(t *testing.T) {
	p := PartInput{BootID: 1, Readings: readings(gen(pidRPM, 0, 9000, 1000, konst(2000)), gen(pidSpeed, 0, 9000, 1000, konst(50)))}
	r := mustAnalyze(t, input(p))
	if len(r.Gears) != 0 || len(r.GearShifts) != 0 {
		t.Fatalf("%+v", r.Gears)
	}
	for _, g := range r.Series.Series.Gear {
		if g != nil {
			t.Fatalf("marcha %d fora da detecção", *g)
		}
	}
}

// Ponto morto fica fora da detecção, e a série mostra 0.
func TestNeutralInSeries(t *testing.T) {
	p := PartInput{BootID: 1, Readings: readings(gen(pidRPM, 0, 29000, 1000, konst(800)), gen(pidSpeed, 0, 29000, 1000, konst(30)))}
	in := input(p)
	in.Params.IdleRPM = f64(700)
	r := mustAnalyze(t, in)
	var zeros int
	for _, g := range r.Series.Series.Gear {
		if g != nil && *g == 0 {
			zeros++
		}
	}
	if zeros != 30 || len(r.Gears) != 0 {
		t.Fatalf("zeros %d marchas %+v", zeros, r.Gears)
	}
	near(t, "ponto morto", r.Summary.NeutralClutchS, 30, 1e-9)
}
