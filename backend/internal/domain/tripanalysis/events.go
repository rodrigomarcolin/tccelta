package tripanalysis

import (
	"math"
	"sort"
)

// gpsPoint é uma posição dentro da janela da viagem, com o índice da parte.
type gpsPoint struct {
	part int
	Position
}

// positions lista as posições dentro da janela de cada parte, em ordem.
func (a *analysis) positions() []gpsPoint {
	var out []gpsPoint
	for k, p := range a.parts {
		for _, ps := range p.positionsIn() {
			out = append(out, gpsPoint{k, ps})
		}
	}
	return out
}

// nearest devolve lat/lon da posição mais próxima de t, se estiver a até GPSGapS de distância.
func (a *analysis) nearest(pts []gpsPoint, t int64) (lat, lon *float64) {
	if len(pts) == 0 {
		return nil, nil
	}
	k := sort.Search(len(pts), func(i int) bool { return pts[i].TMs >= t })
	best := -1
	for _, c := range []int{k - 1, k} {
		if c < 0 || c >= len(pts) {
			continue
		}
		if best < 0 || abs64(pts[c].TMs-t) < abs64(pts[best].TMs-t) {
			best = c
		}
	}
	if float64(abs64(pts[best].TMs-t)) > a.p.GPSGapS*1000 {
		return nil, nil
	}
	return f64(pts[best].Lat), f64(pts[best].Lon)
}

func abs64(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

// runs devolve as sequências contínuas de pontos da janela da parte que atendem à condição.
func runs(p *part, cond func(i int) bool) [][2]int {
	var out [][2]int
	start := -1
	for i := p.first; i <= p.last+1; i++ {
		hit := i <= p.last && cond(i)
		switch {
		case hit && start < 0:
			start = i
		case !hit && start >= 0:
			out = append(out, [2]int{start, i - 1})
			start = -1
		}
	}
	return out
}

func (a *analysis) durationMs(p *part, r [2]int) int64 { return p.t[r[1]] - p.t[r[0]] + a.step }

// events acha acelerações e frenagens bruscas e marchas lentas longas, sem cruzar partes.
func (a *analysis) events(pts []gpsPoint) []Event {
	out := []Event{}
	for _, p := range a.windowed() {
		add := func(typ, unit string, r [2]int, peak float64) {
			e := Event{Type: typ, StartTMs: p.t[r[0]], DurationS: float64(a.durationMs(p, r)) / 1000, Peak: peak, PeakUnit: unit,
				StartSpeedKmh: ptr(p.speed[r[0]]), EndSpeedKmh: ptr(p.speed[r[1]])}
			e.Lat, e.Lon = a.nearest(pts, e.StartTMs)
			out = append(out, e)
		}
		for _, r := range runs(p, func(i int) bool { return !isNaN(p.accel[i]) && p.accel[i] >= a.p.HardAccelMS2 }) {
			if a.durationMs(p, r) >= a.p.EventMinMs {
				peak := math.Inf(-1)
				for i := r[0]; i <= r[1]; i++ {
					peak = math.Max(peak, p.accel[i])
				}
				add(EventHardAccel, "m/s2", r, peak)
			}
		}
		for _, r := range runs(p, func(i int) bool { return !isNaN(p.accel[i]) && p.accel[i] <= a.p.HardBrakeMS2 }) {
			if a.durationMs(p, r) >= a.p.EventMinMs {
				peak := math.Inf(1)
				for i := r[0]; i <= r[1]; i++ {
					peak = math.Min(peak, p.accel[i])
				}
				add(EventHardBrake, "m/s2", r, peak)
			}
		}
		for _, r := range runs(p, func(i int) bool { return p.on[i] && !isNaN(p.speed[i]) && p.speed[i] < a.p.MovingKmh }) {
			if d := a.durationMs(p, r); d >= a.p.LongIdleMs {
				add(EventLongIdle, "s", r, float64(d)/1000)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].StartTMs < out[j].StartTMs })
	return out
}

// stops acha as paradas: sequências com velocidade abaixo de movimento_kmh por pelo menos parada_min_s.
func (a *analysis) stops(pts []gpsPoint) []Stop {
	out := []Stop{}
	for _, p := range a.windowed() {
		for _, r := range runs(p, func(i int) bool { return !isNaN(p.speed[i]) && p.speed[i] < a.p.MovingKmh }) {
			d := a.durationMs(p, r)
			if float64(d) < a.p.StopMinS*1000 {
				continue
			}
			s := Stop{StartTMs: p.t[r[0]], DurationS: float64(d) / 1000, EngineOn: p.on[r[0]]}
			s.Lat, s.Lon = a.nearest(pts, s.StartTMs)
			out = append(out, s)
		}
	}
	return out
}
