package tripanalysis

import (
	"sort"
)

// routeInfo é a rota e o que o GPS informa sobre a qualidade dos dados.
type routeInfo struct {
	route   Route
	n       int       // posições na janela
	gaps    []float64 // intervalos entre posições da mesma parte, em s
	covered float64   // soma dos intervalos até gps_falha_s, em s
	nFail   int
	maxFail float64
	distKm  float64
}

// route monta a rota. A faixa de cada segmento vem da média das leituras brutas de velocidade OBD, nunca da velocidade do GPS.
func (a *analysis) route() routeInfo {
	pts := a.positions()
	ri := routeInfo{n: len(pts), route: emptySeries().Route}
	if len(pts) == 0 {
		return ri
	}
	for _, g := range pts {
		ri.route.TMs = append(ri.route.TMs, g.TMs)
		ri.route.Lat = append(ri.route.Lat, g.Lat)
		ri.route.Lon = append(ri.route.Lon, g.Lon)
	}
	limit := a.p.GPSGapS * 1000
	for k := 0; k+1 < len(pts); k++ {
		x, y := pts[k], pts[k+1]
		same := x.part == y.part
		dtMs := float64(y.TMs - x.TMs)
		if same {
			ri.gaps = append(ri.gaps, dtMs/1000)
			if dtMs <= limit {
				ri.covered += dtMs / 1000
				ri.distKm += haversineKm(x.Lat, x.Lon, y.Lat, y.Lon)
			} else {
				ri.nFail++
				ri.maxFail = max(ri.maxFail, dtMs/1000)
			}
		}
		if !same || dtMs > limit {
			ri.route.Segments.SpeedKmh = append(ri.route.Segments.SpeedKmh, nil)
			ri.route.Segments.Band = append(ri.route.Segments.Band, BandGPSFailure)
			continue
		}
		v, ok := a.segmentSpeed(a.parts[x.part], x.TMs, y.TMs)
		if !ok {
			ri.route.Segments.SpeedKmh = append(ri.route.Segments.SpeedKmh, nil)
			ri.route.Segments.Band = append(ri.route.Segments.Band, BandNoSpeed)
			continue
		}
		ri.route.Segments.SpeedKmh = append(ri.route.Segments.SpeedKmh, f64(v))
		ri.route.Segments.Band = append(ri.route.Segments.Band, BandID(v, a.p.MovingKmh))
	}
	return ri
}

// segmentSpeed é a média das leituras brutas de velocidade em [t0, t1); sem nenhuma, a última leitura se tiver até
// velocidade_idade_max_ms de idade.
func (a *analysis) segmentSpeed(p *part, t0, t1 int64) (float64, bool) {
	s, ok := p.raw[pidSpeed]
	if !ok {
		return 0, false
	}
	i := sort.Search(len(s.t), func(k int) bool { return s.t[k] >= t0 })
	j := sort.Search(len(s.t), func(k int) bool { return s.t[k] >= t1 })
	if j > i {
		var sum float64
		for _, v := range s.v[i:j] {
			sum += v
		}
		return sum / float64(j-i), true
	}
	if i > 0 && t0-s.t[i-1] <= a.p.SpeedMaxAgeMs {
		return s.v[i-1], true
	}
	return 0, false
}

// gpsQuality preenche qualidade.gps. Sem posições, a cobertura é 0 e o resto é nulo.
func (a *analysis) gpsQuality(s *Summary, ri routeInfo) {
	if ri.n == 0 {
		s.GPSCoveragePct = f64(0)
		return
	}
	s.GPSPeriodS = ptr(median(ri.gaps))
	if s.DurationS != nil && *s.DurationS > 0 {
		s.GPSCoveragePct = f64(min(100, ri.covered / *s.DurationS * 100))
	}
	s.GPSNGaps, s.GPSMaxGapS, s.GPSDistanceKm = i32(ri.nFail), f64(ri.maxFail), f64(ri.distKm)
	if s.DistanceKm != nil && *s.DistanceKm > 0 {
		s.GPSDistanceDiffPct = f64((ri.distKm - *s.DistanceKm) / *s.DistanceKm * 100)
	}
}
