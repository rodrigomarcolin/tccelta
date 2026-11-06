package trip

import (
	"math"
	"testing"

	"tccelta/backend/internal/domain/tripanalysis"
)

// lineRoute é uma rota reta de n pontos, todos os segmentos com a faixa dada.
func lineRoute(n int, band string) tripanalysis.Route {
	r := tripanalysis.Route{}
	for i := 0; i < n; i++ {
		r.TMs = append(r.TMs, int64(i)*1000)
		r.Lat = append(r.Lat, -23.5-float64(i)*0.0001)
		r.Lon = append(r.Lon, -46.6-float64(i)*0.0001)
		if i > 0 {
			r.Segments.Band = append(r.Segments.Band, band)
		}
	}
	return r
}

func total(p [][]Point) int {
	n := 0
	for _, s := range p {
		n += len(s)
	}
	return n
}

func TestRoutePreviewEmpty(t *testing.T) {
	if RoutePreview(tripanalysis.Route{}) != nil {
		t.Fatal("sem pontos o resumo é nulo")
	}
}

func TestRoutePreviewStraightLineKeepsEnds(t *testing.T) {
	p := RoutePreview(lineRoute(50, "v1"))
	if len(p) != 1 || len(p[0]) != 2 {
		t.Fatalf("uma reta se resume às pontas: %v", p)
	}
	if p[0][0] != (Point{-23.5, -46.6}) || p[0][1] != (Point{-23.5049, -46.6049}) {
		t.Fatalf("%v", p[0])
	}
}

func TestRoutePreviewKeepsCorner(t *testing.T) {
	r := tripanalysis.Route{Lat: []float64{0, 0, 0, 0.01, 0.02}, Lon: []float64{0, 0.01, 0.02, 0.02, 0.02},
		Segments: tripanalysis.RouteSegments{Band: []string{"v1", "v1", "v1", "v1"}}}
	p := RoutePreview(r)
	if len(p) != 1 || len(p[0]) != 3 || p[0][1] != (Point{0, 0.02}) {
		t.Fatalf("a esquina precisa ficar: %v", p)
	}
}

func TestRoutePreviewCutsAtGPSFailure(t *testing.T) {
	// 6 pontos: o segmento 2 (entre os pontos 2 e 3) é falha_gps. A faixa tem um item por par de pontos (n-1).
	r := tripanalysis.Route{Lat: []float64{0, 0.01, 0.02, 0.5, 0.51, 0.52}, Lon: []float64{0, 0, 0, 0.5, 0.5, 0.5},
		Segments: tripanalysis.RouteSegments{Band: []string{"v1", "v2", tripanalysis.BandGPSFailure, "v1", "sem_velocidade"}}}
	p := RoutePreview(r)
	if len(p) != 2 || p[0][len(p[0])-1] != (Point{0.02, 0}) || p[1][0] != (Point{0.5, 0.5}) {
		t.Fatalf("%v", p)
	}
	// sem_velocidade não corta
	r.Segments.Band[2] = "sem_velocidade"
	if p := RoutePreview(r); len(p) != 1 {
		t.Fatalf("%v", p)
	}
}

func TestRoutePreviewDropsIsolatedPoints(t *testing.T) {
	// o ponto 2 fica isolado entre duas falhas: não desenha linha e some
	r := tripanalysis.Route{Lat: []float64{0, 0.01, 0.5, 1, 1.01}, Lon: []float64{0, 0, 0.5, 1, 1},
		Segments: tripanalysis.RouteSegments{Band: []string{"v1", tripanalysis.BandGPSFailure, tripanalysis.BandGPSFailure, "v1"}}}
	if p := RoutePreview(r); len(p) != 2 || len(p[0]) != 2 || len(p[1]) != 2 {
		t.Fatalf("%v", p)
	}
	// todos os segmentos são falha: sobra um ponto
	r.Segments.Band = []string{tripanalysis.BandGPSFailure, tripanalysis.BandGPSFailure, tripanalysis.BandGPSFailure, tripanalysis.BandGPSFailure}
	if p := RoutePreview(r); len(p) != 1 || len(p[0]) != 1 {
		t.Fatalf("%v", p)
	}
}

func TestRoutePreviewStoppedTrip(t *testing.T) {
	r := tripanalysis.Route{Lat: []float64{-23.123456789, -23.123456789, -23.123456789}, Lon: []float64{-46.1, -46.1, -46.1},
		Segments: tripanalysis.RouteSegments{Band: []string{"parado", "parado"}}}
	p := RoutePreview(r)
	if len(p) != 1 || len(p[0]) != 1 || p[0][0] != (Point{-23.12346, -46.1}) {
		t.Fatalf("diagonal zero vira um ponto, com 5 casas: %v", p)
	}
	one := tripanalysis.Route{TMs: []int64{0}, Lat: []float64{1}, Lon: []float64{2}}
	if p := RoutePreview(one); len(p) != 1 || len(p[0]) != 1 {
		t.Fatalf("%v", p)
	}
}

func TestRoutePreviewLimitsPoints(t *testing.T) {
	// ziguezague denso: nenhum ponto é descartável com a tolerância inicial
	r := tripanalysis.Route{}
	for i := 0; i < 3000; i++ {
		r.Lat = append(r.Lat, -23.5+0.00001*float64(i))
		r.Lon = append(r.Lon, -46.6+0.01*math.Sin(float64(i)/7))
		if i > 0 {
			r.Segments.Band = append(r.Segments.Band, "v1")
		}
	}
	p := RoutePreview(r)
	if n := total(p); n > MaxPreviewPoints || n < 10 {
		t.Fatalf("%d pontos", n)
	}
	// 5 casas decimais
	for _, s := range p {
		for _, pt := range s {
			if v := pt[0] * 1e5; math.Abs(v-math.Round(v)) > 1e-6 {
				t.Fatalf("%v", pt)
			}
		}
	}
}

func TestRoutePreviewManyRunsStaysWithinBudget(t *testing.T) {
	// 200 trechos de 2 pontos: o orçamento só comporta 60
	r := tripanalysis.Route{}
	for i := 0; i < 200; i++ {
		r.Lat = append(r.Lat, float64(i)*0.01, float64(i)*0.01+0.005)
		r.Lon = append(r.Lon, 0, 0)
		r.Segments.Band = append(r.Segments.Band, "v1", tripanalysis.BandGPSFailure)
	}
	r.Segments.Band = r.Segments.Band[:len(r.Lat)-1]
	if p := RoutePreview(r); total(p) > MaxPreviewPoints || len(p) == 0 {
		t.Fatalf("%d pontos em %d trechos", total(p), len(p))
	}
}

func TestRoutePreviewShorterBandIsTolerated(t *testing.T) {
	r := lineRoute(10, "v1")
	r.Segments.Band = r.Segments.Band[:3] // faixa menor que n-1: o resto é "sem falha"
	if p := RoutePreview(r); len(p) != 1 {
		t.Fatalf("%v", p)
	}
}
