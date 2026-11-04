package tripanalysis

import "math"

// idleRPM é a rotação de marcha lenta: a informada, a mediana dos pontos parados com motor ligado, ou a reserva.
func (a *analysis) idleRPM() (float64, string) {
	if a.p.IdleRPM != nil {
		return *a.p.IdleRPM, "informada"
	}
	var rpms []float64
	a.eachPoint(func(p *part, i int) {
		if p.on[i] && !isNaN(p.speed[i]) && p.speed[i] < a.p.MovingKmh {
			rpms = append(rpms, p.rpm[i])
		}
	})
	if len(rpms) < 10 {
		return a.p.IdleFallbackRPM, "reserva"
	}
	return median(rpms), "estimada"
}

// summarize calcula resumo, condutor, motor, marchas (marcha lenta) e qualidade, nos pontos da janela da viagem.
// Eventos, GPS e marchas entram depois (fillEventTotals, gpsQuality, shiftIndicators).
func (a *analysis) summarize(idle float64, origin string) Summary {
	pr, dt := a.p, a.dt
	var (
		nPts, nSpeed, nMoving, nStopped, nOn, nIdle, nMAF int
		nPedalHigh, nCold, nBrake, nNeutral               int
		dist, speedSum, fuelL, air, pedSum                float64
		pedN                                              int
		rmsSum                                            float64
		rmsN                                              int
		hasRPM, hasMAF, hasCoolant, hasPedal, hasSpeed    = a.hasPID(pidRPM), a.hasPID(pidMAF), a.hasPID(pidCoolant), a.hasPID(pidPedal), a.hasPID(pidSpeed)
	)
	a.eachPoint(func(p *part, i int) {
		nPts++
		v, r, ped, cool, maf := p.speed[i], p.rpm[i], p.pedal[i], p.coolant[i], p.maf[i]
		on := p.on[i]
		if p.gear[i] == 0 {
			nNeutral++
		}
		moving := false
		if !isNaN(v) {
			nSpeed++
			dist += v * dt / 3600
			if v >= pr.MovingKmh {
				moving = true
				nMoving++
				speedSum += v
				if !isNaN(ped) {
					pedSum += ped
					pedN++
				}
				if !isNaN(p.accel[i]) {
					rmsSum += p.accel[i] * p.accel[i]
					rmsN++
				}
			} else {
				nStopped++
			}
		}
		if on {
			nOn++
			if !isNaN(v) && v < pr.MovingKmh {
				nIdle++
			}
			if !isNaN(ped) && ped >= pr.PedalHighPct {
				nPedalHigh++
			}
			if !isNaN(cool) && cool < pr.ColdCoolantC && (r > pr.ColdRPM || (!isNaN(ped) && ped > pr.ColdPedalPct)) {
				nCold++
			}
			if moving && !isNaN(ped) && ped <= pr.EngineBrakePedalPct && r >= pr.EngineBrakeRPM && v >= pr.EngineBrakeKmh {
				nBrake++
			}
		}
		if !isNaN(maf) {
			nMAF++
			fuelL += maf / pr.AFR / pr.DensityGL * 3600 * dt / 3600
			air += maf * dt
		}
	})

	var s Summary
	ws := a.windowed()
	first, last := ws[0], ws[len(ws)-1]
	st, en := first.t[first.first], last.t[last.last]
	s.StartTMs, s.EndTMs = &st, &en
	var dur float64
	for _, p := range ws {
		dur += float64(p.t[p.last]-p.t[p.first]) / 1000
	}
	s.DurationS = f64(dur)
	s.NoDataS = f64(float64(en-st)/1000 - dur)

	if hasSpeed && nSpeed > 0 {
		s.DistanceKm = f64(dist)
		s.MovingS = f64(float64(nMoving) * dt)
		s.StoppedS = f64(float64(nStopped) * dt)
		if nMoving > 0 {
			s.AvgSpeedKmh = f64(speedSum / float64(nMoving))
		}
		s.MaxSpeedKmh = a.maxSpeed()
		if hasPedal && pedN > 0 {
			s.PedalMeanPct = f64(pedSum / float64(pedN))
		}
		if rmsN > 0 {
			s.AccelRMSMS2 = f64(math.Sqrt(rmsSum / float64(rmsN)))
		}
		s.CruiseS = f64(a.cruiseSeconds())
		if hasPedal && nMoving > 0 {
			s.EngineBrakePct = f64(float64(nBrake) / float64(nMoving) * 100)
		}
	}
	if hasRPM {
		s.EngineOnS = f64(float64(nOn) * dt)
		if hasSpeed && nSpeed > 0 {
			s.IdleS = f64(float64(nIdle) * dt)
			if nOn > 0 {
				s.IdlePct = f64(float64(nIdle) / float64(nOn) * 100)
			}
		}
		if hasPedal {
			s.PedalHighS = f64(float64(nPedalHigh) * dt)
		}
	}
	if hasMAF {
		s.FuelL = f64(fuelL)
		s.IntakeAirG = f64(air)
		if nPts > 0 {
			s.FuelCoveragePct = f64(float64(nMAF) / float64(nPts) * 100)
		}
		if s.DistanceKm != nil && fuelL > 0.02 {
			s.KmPerL = f64(*s.DistanceKm / fuelL)
		}
	}
	if hasCoolant {
		s.ColdLoadS = f64(float64(nCold) * dt)
		s.WarmupS, s.MaxCoolantC = a.coolantStats(st)
	}
	s.PedalRateP95PctS = a.pedalRateP95()
	s.IdleRPM, s.IdleRPMOrigin, s.IdleMarginRPM = f64(idle), &origin, f64(pr.IdleMarginRPM)
	if hasSpeed && hasRPM {
		s.NeutralClutchS = f64(float64(nNeutral) * dt)
	}
	s.PIDPeriodsMs = map[string]float64{}
	for pid, iv := range a.intervals {
		s.PIDPeriodsMs[pidName(pid)] = iv
	}
	var dataS float64
	for _, p := range a.parts {
		s.NReadings += p.nSamples
		dataS += float64(p.dataEnd-p.dataStart) / 1000
	}
	if dataS > 0 {
		s.ReadingsPerS = f64(float64(s.NReadings) / dataS)
	}
	return s
}

// maxSpeed é o máximo das leituras brutas de velocidade dentro das janelas.
func (a *analysis) maxSpeed() *float64 {
	m, ok := math.Inf(-1), false
	for _, p := range a.windowed() {
		_, v := p.rawIn(pidSpeed)
		for _, x := range v {
			m, ok = math.Max(m, x), true
		}
	}
	if !ok {
		return nil
	}
	return &m
}

// coolantStats devolve o tempo até a primeira leitura aquecida (desde o início da viagem) e a temperatura máxima.
func (a *analysis) coolantStats(startT int64) (warmup, maxC *float64) {
	m, ok := math.Inf(-1), false
	warm := math.NaN()
	for _, p := range a.windowed() {
		t, v := p.rawIn(pidCoolant)
		for k, x := range v {
			m, ok = math.Max(m, x), true
			if isNaN(warm) && x >= a.p.WarmCoolantC {
				warm = math.Max(0, float64(t[k]-startT)/1000)
			}
		}
	}
	if ok {
		maxC = &m
	}
	return ptr(warm), maxC
}

// pedalRateP95 é o p95 de |Δpedal|/Δt entre leituras brutas consecutivas (Δt até 3x o intervalo mediano), dentro de cada parte.
func (a *analysis) pedalRateP95() *float64 {
	iv, ok := a.intervals[pidPedal]
	if !ok {
		return nil
	}
	var rates []float64
	for _, p := range a.windowed() {
		t, v := p.rawIn(pidPedal)
		for k := 1; k < len(t); k++ {
			d := float64(t[k] - t[k-1])
			if d > 0 && d <= 3*iv {
				rates = append(rates, math.Abs(v[k]-v[k-1])/(d/1000))
			}
		}
	}
	return ptr(quantile(rates, 0.95))
}

// cruiseSeconds soma os trechos de velocidade quase constante (spec 02, "Trechos de cruzeiro"), parte a parte.
func (a *analysis) cruiseSeconds() float64 {
	pr := a.p
	var total float64
	for _, p := range a.windowed() {
		elig := func(i int) bool { return !isNaN(p.speed[i]) && p.speed[i] >= pr.CruiseMinKmh }
		for i := p.first; i <= p.last; {
			if !elig(i) {
				i++
				continue
			}
			lo, hi := p.speed[i], p.speed[i]
			j := i
			for j+1 <= p.last && elig(j+1) {
				nlo, nhi := math.Min(lo, p.speed[j+1]), math.Max(hi, p.speed[j+1])
				if nhi-nlo > pr.CruiseAmplitudeKmh {
					break
				}
				lo, hi = nlo, nhi
				j++
			}
			durMs := p.t[j] - p.t[i] + a.step
			if float64(durMs) >= pr.CruiseMinS*1000 {
				total += float64(durMs) / 1000
				i = j + 1
			} else {
				i++
			}
		}
	}
	return total
}

// fillEventTotals preenche as contagens de eventos e as taxas por 100 km.
func (a *analysis) fillEventTotals(s *Summary, events []Event) {
	var acc, brk int
	for _, e := range events {
		switch e.Type {
		case EventHardAccel:
			acc++
		case EventHardBrake:
			brk++
		}
	}
	if s.DistanceKm == nil {
		return
	}
	s.NHardAccel, s.NHardBrake = i32(acc), i32(brk)
	if *s.DistanceKm >= a.p.RateMinKm {
		s.HardAccelPer100Km = f64(float64(acc) * 100 / *s.DistanceKm)
		s.HardBrakePer100Km = f64(float64(brk) * 100 / *s.DistanceKm)
	}
}

// shiftIndicators preenche mediana e quartis de rpm_antes nas subidas e a mediana nas reduções.
func (a *analysis) shiftIndicators(s *Summary, shifts []GearShift) {
	var ups, downs []float64
	for _, sh := range shifts {
		if sh.Direction == ShiftUp {
			ups = append(ups, sh.RPMBefore)
		} else {
			downs = append(downs, sh.RPMBefore)
		}
	}
	if len(ups) > 0 {
		s.UpshiftRPMMedian, s.UpshiftRPMP25, s.UpshiftRPMP75 = ptr(median(ups)), ptr(quantile(ups, 0.25)), ptr(quantile(ups, 0.75))
	}
	s.UpshiftN = i32(len(ups))
	if len(downs) > 0 {
		s.DownshiftRPMMedian = ptr(median(downs))
	}
	s.DownshiftN = i32(len(downs))
}

// Faixas de velocidade (spec 02). parado usa movimento_kmh como limite superior.
var bandLimits = []struct {
	id       string
	from, to float64
}{{"ate_30", 0, 30}, {"30_60", 30, 60}, {"60_80", 60, 80}, {"80_110", 80, 110}, {"acima_110", 110, math.Inf(1)}}

// BandID devolve a faixa de uma velocidade.
func BandID(v, movingKmh float64) string {
	if v < movingKmh {
		return "parado"
	}
	for _, b := range bandLimits {
		if v < b.to {
			return b.id
		}
	}
	return "acima_110"
}

// speedBands soma tempo e distância por faixa; sem velocidade, não há faixas.
func (a *analysis) speedBands() []SpeedBand {
	bands := []SpeedBand{{ID: "parado", FromKmh: 0, ToKmh: f64(a.p.MovingKmh)}}
	for _, b := range bandLimits {
		sb := SpeedBand{ID: b.id, FromKmh: b.from}
		if b.id == "ate_30" {
			sb.FromKmh = a.p.MovingKmh
		}
		if !math.IsInf(b.to, 1) {
			sb.ToKmh = f64(b.to)
		}
		bands = append(bands, sb)
	}
	idx := map[string]int{}
	for i, b := range bands {
		idx[b.ID] = i
	}
	seen := false
	var movingKm float64
	a.eachPoint(func(p *part, i int) {
		v := p.speed[i]
		if isNaN(v) {
			return
		}
		seen = true
		b := &bands[idx[BandID(v, a.p.MovingKmh)]]
		b.TimeS += a.dt
		b.DistanceKm += v * a.dt / 3600
		if b.ID != "parado" {
			movingKm += v * a.dt / 3600
		}
	})
	if !seen {
		return []SpeedBand{}
	}
	if movingKm > 0 {
		for i := range bands {
			if bands[i].ID != "parado" {
				bands[i].DistancePct = f64(bands[i].DistanceKm / movingKm * 100)
			}
		}
	}
	return bands
}
