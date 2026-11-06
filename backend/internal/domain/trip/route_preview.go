package trip

import (
	"math"

	"tccelta/backend/internal/domain/tripanalysis"
)

// Limites do resumo da rota da listagem.
const (
	// MaxPreviewPoints é o total de pontos de todos os trechos do resumo.
	MaxPreviewPoints = 120
	// previewTolerance é a tolerância inicial do Douglas-Peucker, relativa à diagonal do trajeto.
	previewTolerance = 0.004
	previewDecimals  = 1e5
)

// Point é uma posição [lat, lon] no resumo da rota.
type Point [2]float64

// RoutePreview simplifica a rota para o minimapa da listagem. Devolve trechos, cortados onde segmentos.faixa é falha_gps
// (para não desenhar reta sobre um buraco de GPS), com no máximo MaxPreviewPoints pontos no total e 5 casas decimais. Sem
// pontos devolve nil. Um trajeto parado (diagonal zero) vira um único trecho de um só ponto.
func RoutePreview(r tripanalysis.Route) [][]Point {
	n := min(len(r.Lat), len(r.Lon))
	if n == 0 {
		return nil
	}
	minLat, maxLat, minLon, maxLon := r.Lat[0], r.Lat[0], r.Lon[0], r.Lon[0]
	for i := 1; i < n; i++ {
		minLat, maxLat = min(minLat, r.Lat[i]), max(maxLat, r.Lat[i])
		minLon, maxLon = min(minLon, r.Lon[i]), max(maxLon, r.Lon[i])
	}
	// A longitude encolhe com a latitude; sem esse fator a tolerância seria anisotrópica.
	kx := math.Cos((minLat + maxLat) / 2 * math.Pi / 180)
	diag := math.Hypot((maxLon-minLon)*kx, maxLat-minLat)
	if diag == 0 {
		return [][]Point{{roundPoint(r.Lat[0], r.Lon[0])}}
	}

	// Cada segmento i liga os pontos i e i+1; a faixa falha_gps o separa em dois trechos.
	var runs [][]xy
	var cur []xy
	for i := 0; i < n; i++ {
		cur = append(cur, xy{x: r.Lon[i] * kx, y: r.Lat[i], lat: r.Lat[i], lon: r.Lon[i]})
		if i+1 == n || (i < len(r.Segments.Band) && r.Segments.Band[i] == tripanalysis.BandGPSFailure) {
			if len(cur) > 1 { // um ponto isolado não desenha linha
				runs = append(runs, cur)
			}
			cur = nil
		}
	}
	if len(runs) == 0 { // todos os pontos estão isolados por falhas: sobra o primeiro, como ponto
		return [][]Point{{roundPoint(r.Lat[0], r.Lon[0])}}
	}

	// Aumenta a tolerância até caber no orçamento de pontos.
	var out [][]xy
	for tol := diag * previewTolerance; ; tol *= 1.5 {
		out = out[:0]
		total := 0
		for _, run := range runs {
			s := simplify(run, tol)
			out = append(out, s)
			total += len(s)
		}
		if total <= MaxPreviewPoints || tol > diag {
			break
		}
	}
	// Com trechos demais, nem a tolerância máxima (2 pontos por trecho) cabe: mantém os primeiros que cabem.
	budget := MaxPreviewPoints
	res := make([][]Point, 0, len(out))
	for _, s := range out {
		if len(s) > budget {
			break
		}
		budget -= len(s)
		pts := make([]Point, len(s))
		for i, p := range s {
			pts[i] = roundPoint(p.lat, p.lon)
		}
		res = append(res, pts)
	}
	return res
}

type xy struct{ x, y, lat, lon float64 }

func roundPoint(lat, lon float64) Point {
	return Point{math.Round(lat*previewDecimals) / previewDecimals, math.Round(lon*previewDecimals) / previewDecimals}
}

// simplify é o Douglas-Peucker iterativo (sem recursão, para trechos longos). Mantém sempre as pontas.
func simplify(pts []xy, tol float64) []xy {
	keep := make([]bool, len(pts))
	keep[0], keep[len(pts)-1] = true, true
	stack := [][2]int{{0, len(pts) - 1}}
	for len(stack) > 0 {
		lo, hi := stack[len(stack)-1][0], stack[len(stack)-1][1]
		stack = stack[:len(stack)-1]
		far, farD := -1, tol
		for i := lo + 1; i < hi; i++ {
			if d := distToSegment(pts[i], pts[lo], pts[hi]); d > farD {
				far, farD = i, d
			}
		}
		if far >= 0 {
			keep[far] = true
			stack = append(stack, [2]int{lo, far}, [2]int{far, hi})
		}
	}
	out := make([]xy, 0, len(pts))
	for i, p := range pts {
		if keep[i] {
			out = append(out, p)
		}
	}
	return out
}

func distToSegment(p, a, b xy) float64 {
	dx, dy := b.x-a.x, b.y-a.y
	l2 := dx*dx + dy*dy
	if l2 == 0 {
		return math.Hypot(p.x-a.x, p.y-a.y)
	}
	t := max(0, min(1, ((p.x-a.x)*dx+(p.y-a.y)*dy)/l2))
	return math.Hypot(p.x-(a.x+t*dx), p.y-(a.y+t*dy))
}
