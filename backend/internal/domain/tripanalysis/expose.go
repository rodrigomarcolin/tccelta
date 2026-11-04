package tripanalysis

import "math"

// Células da matriz rotação x pedal.
const (
	rpmBin   = 500
	pedalBin = 10
)

var rpmEdges = []float64{0, 1000, 1500, 2000, 2500, 3000, 3500, 4000, 5000}

var pedalEdges = []struct {
	from, to float64
	label    string
}{{0, 5, "Solto"}, {5, 20, ""}, {20, 40, ""}, {40, 60, ""}, {60, 80, ""}, {80, math.Inf(1), ""}}

// distributions soma o tempo com motor ligado por faixa de rotação, de pedal e por célula rotação x pedal.
// Sem o PID, a distribuição correspondente vem vazia.
func (a *analysis) distributions() ([]RPMBand, []PedalBand, RPMPedalMatrix) {
	hasRPM, hasPedal := a.hasPID(pidRPM), a.hasPID(pidPedal)
	rpmBands := make([]RPMBand, len(rpmEdges))
	for i, e := range rpmEdges {
		rpmBands[i].From = e
		if i+1 < len(rpmEdges) {
			rpmBands[i].To = f64(rpmEdges[i+1])
		}
	}
	pedalBands := make([]PedalBand, len(pedalEdges))
	for i, e := range pedalEdges {
		pedalBands[i] = PedalBand{From: e.from, To: math.Min(e.to, 100)}
		if e.label != "" {
			l := e.label
			pedalBands[i].Label = &l
		}
	}
	var rpmMax float64
	a.eachPoint(func(p *part, i int) {
		if !isNaN(p.rpm[i]) {
			rpmMax = math.Max(rpmMax, p.rpm[i])
		}
	})
	rows := max(1, int(math.Ceil((rpmMax+1)/rpmBin)))
	matrix := make([][]float64, rows)
	for i := range matrix {
		matrix[i] = make([]float64, 10)
	}
	a.eachPoint(func(p *part, i int) {
		if !p.on[i] {
			return
		}
		r, ped := p.rpm[i], p.pedal[i]
		for k := len(rpmEdges) - 1; k >= 0; k-- {
			if r >= rpmEdges[k] {
				rpmBands[k].TimeS += a.dt
				break
			}
		}
		if isNaN(ped) {
			return
		}
		for k := len(pedalEdges) - 1; k >= 0; k-- {
			if ped >= pedalEdges[k].from {
				pedalBands[k].TimeS += a.dt
				break
			}
		}
		matrix[min(rows-1, int(r/rpmBin))][min(9, int(ped/pedalBin))] += a.dt
	})
	var rpmTotal, pedalTotal float64
	for _, b := range rpmBands {
		rpmTotal += b.TimeS
	}
	for _, b := range pedalBands {
		pedalTotal += b.TimeS
	}
	for i := range rpmBands {
		if rpmTotal > 0 {
			rpmBands[i].Pct = rpmBands[i].TimeS / rpmTotal * 100
		}
	}
	for i := range pedalBands {
		if pedalTotal > 0 {
			pedalBands[i].Pct = pedalBands[i].TimeS / pedalTotal * 100
		}
	}
	m := RPMPedalMatrix{RPMBin: rpmBin, PedalBin: pedalBin, TimeS: matrix}
	if !hasRPM {
		return []RPMBand{}, []PedalBand{}, RPMPedalMatrix{RPMBin: rpmBin, PedalBin: pedalBin, TimeS: [][]float64{}}
	}
	if !hasPedal {
		return rpmBands, []PedalBand{}, RPMPedalMatrix{RPMBin: rpmBin, PedalBin: pedalBin, TimeS: [][]float64{}}
	}
	return rpmBands, pedalBands, m
}

// series monta as séries da grade (reduzidas a serie_max_pontos, com um separador nulo após cada parte) e as séries lentas.
func (a *analysis) series(res *Result) {
	out := emptySeries()
	ws := a.windowed()
	total := 0
	for _, p := range ws {
		total += p.last - p.first + 1
	}
	stride := 1
	if total > a.p.SeriesMaxPoints {
		stride = int(math.Ceil(float64(total) / float64(a.p.SeriesMaxPoints)))
	}
	out.StepMs, out.ExposureStepMs = int32(a.step), int32(a.step)*int32(stride)

	var fuelCum, distCum float64
	s := &out.Series
	for _, p := range ws {
		for i := p.first; i <= p.last; i++ {
			lph := nan
			if !isNaN(p.maf[i]) {
				lph = p.maf[i] / a.p.AFR / a.p.DensityGL * 3600
				fuelCum += lph * a.dt / 3600
			}
			if !isNaN(p.speed[i]) {
				distCum += p.speed[i] * a.dt / 3600
			}
			if (i-p.first)%stride != 0 {
				continue
			}
			s.TMs = append(s.TMs, p.t[i])
			s.Speed = append(s.Speed, valPtr(p.speed[i]))
			s.RPM = append(s.RPM, valPtr(p.rpm[i]))
			rk := nan
			if !isNaN(p.rpm[i]) && !isNaN(p.speed[i]) && p.speed[i] > 0 {
				rk = p.rpm[i] / p.speed[i]
			}
			s.RPMPerKmh = append(s.RPMPerKmh, valPtr(rk))
			var g *int
			if p.gear[i] >= 0 {
				v := p.gear[i]
				g = &v
			}
			s.Gear = append(s.Gear, g)
			s.Pedal = append(s.Pedal, valPtr(p.pedal[i]))
			s.Accel = append(s.Accel, valPtr(p.accel[i]))
			s.Load = append(s.Load, valPtr(p.load[i]))
			s.FuelLH = append(s.FuelLH, valPtr(lph))
			s.FuelCumL = append(s.FuelCumL, f64(fuelCum))
			s.DistCumKm = append(s.DistCumKm, f64(distCum))
		}
		// separador: valores nulos logo após o fim da parte
		s.TMs = append(s.TMs, p.t[p.last]+a.step)
		s.Speed, s.RPM, s.RPMPerKmh, s.Gear = append(s.Speed, nil), append(s.RPM, nil), append(s.RPMPerKmh, nil), append(s.Gear, nil)
		s.Pedal, s.Accel, s.Load = append(s.Pedal, nil), append(s.Accel, nil), append(s.Load, nil)
		s.FuelLH, s.FuelCumL, s.DistCumKm = append(s.FuelLH, nil), append(s.FuelCumL, nil), append(s.DistCumKm, nil)
	}
	a.slowSeries(&out.Slow)
	// os campos preenchidos antes (rota, distribuições) já estão em res.Series
	out.Route, out.RPMDistribution, out.PedalDistribution, out.RPMPedal = res.Series.Route, res.Series.RPMDistribution, res.Series.PedalDistribution, res.Series.RPMPedal
	res.Series = out
}

// slowSeries copia a temperatura e o nível do tanque (leituras brutas) com a mediana móvel de ±3 leituras do tanque.
func (a *analysis) slowSeries(s *SlowSeries) {
	for _, p := range a.windowed() {
		t, v := p.rawIn(pidCoolant)
		for k := range t {
			s.Temperature.TMs = append(s.Temperature.TMs, t[k])
			s.Temperature.C = append(s.Temperature.C, f64(v[k]))
		}
		if len(t) > 0 {
			s.Temperature.TMs = append(s.Temperature.TMs, p.t[p.last])
			s.Temperature.C = append(s.Temperature.C, nil)
		}
		t, v = p.rawIn(pidTank)
		for k := range t {
			lo, hi := max(0, k-3), min(len(v)-1, k+3)
			s.Tank.TMs = append(s.Tank.TMs, t[k])
			s.Tank.Pct = append(s.Tank.Pct, f64(v[k]))
			s.Tank.MedianPct = append(s.Tank.MedianPct, f64(median(v[lo:hi+1])))
		}
		if len(t) > 0 {
			s.Tank.TMs = append(s.Tank.TMs, p.t[p.last])
			s.Tank.Pct = append(s.Tank.Pct, nil)
			s.Tank.MedianPct = append(s.Tank.MedianPct, nil)
		}
	}
}
