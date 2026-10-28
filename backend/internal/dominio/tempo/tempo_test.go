package tempo

import (
	"math"
	"math/rand"
	"testing"
	"time"
)

func ms(h, m int, s float64) float64 {
	return float64(time.Date(2026, 10, 26, h, m, 0, 0, time.UTC).UnixMilli()) + s*1000
}

func TestExemploDoDoc(t *testing.T) {
	c := ConfigPadrao()
	e := EstimarPorAncoras([]Ancora{
		{TMs: 600000, S1Ms: ms(10, 0, 0), S2Ms: ms(10, 0, 0.8)},
		{TMs: 900000, S1Ms: ms(10, 5, 0.4), S2Ms: ms(10, 5, 1.0)},
	}, c)
	if len(e.Grupos) != 1 || e.B != 0 || e.IncertezaBaseMs != 200 {
		t.Fatalf("%v", e)
	}
	g := e.Grupos[0]
	if g.Lo != ms(9, 50, 0.4) || g.Hi != ms(9, 50, 0.8) || e.AMs != ms(9, 50, 0.6) {
		t.Fatalf("lo=%v hi=%v a=%v", g.Lo, g.Hi, e.AMs)
	}
}

func TestIntersecaoVazia(t *testing.T) {
	e := EstimarPorAncoras([]Ancora{
		{TMs: 0, S1Ms: 1000, S2Ms: 1100},
		{TMs: 60000, S1Ms: 61500, S2Ms: 61600},
	}, ConfigPadrao())
	if len(e.Grupos) != 2 || len(e.Alertas) != 1 || e.Alertas[0].Tipo != "intersecao_vazia" {
		t.Fatalf("%v %v", e, e.Alertas)
	}
}

func sintetica(ppm float64, horas int, ruido float64, seed int64) []Ancora {
	r := rand.New(rand.NewSource(seed)) //nolint:gosec // G404: semente fixa de teste
	var out []Ancora
	for t := int64(0); t <= int64(horas)*3600000; t += 1800000 {
		o := 1.7e12 + ppm*1e-6*float64(t)
		out = append(out, Ancora{TMs: t, S1Ms: o + float64(t) - ruido*r.Float64(), S2Ms: o + float64(t) + ruido*r.Float64()})
	}
	return out
}

func TestDerivaConhecida(t *testing.T) {
	e := EstimarPorAncoras(sintetica(20, 6, 100, 1), ConfigPadrao())
	if math.Abs(e.B*1e6-20) > 5 || len(e.Alertas) != 0 {
		t.Fatalf("%v %v", e, e.Alertas)
	}
}

func TestDerivaExcessiva(t *testing.T) {
	e := EstimarPorAncoras(sintetica(60, 6, 100, 2), ConfigPadrao())
	achou := false
	for _, a := range e.Alertas {
		achou = achou || a.Tipo == "deriva_excessiva"
	}
	if !achou {
		t.Fatalf("%v", e)
	}
}

func TestGruposProximos(t *testing.T) {
	e := EstimarPorAncoras([]Ancora{
		{TMs: 0, S1Ms: 1000, S2Ms: 1400},
		{TMs: 700000, S1Ms: 701300, S2Ms: 701700},
	}, ConfigPadrao())
	if len(e.Grupos) != 2 || e.B != 0 || len(e.Alertas) != 0 {
		t.Fatalf("%v %v", e, e.Alertas)
	}
}

func TestJanelaNaoEncadeia(t *testing.T) {
	var a []Ancora
	for i := int64(0); i <= 15; i++ {
		a = append(a, Ancora{TMs: i * 240000, S1Ms: 1e12, S2Ms: 1e12 + 100})
		a[i].S1Ms += float64(a[i].TMs)
		a[i].S2Ms += float64(a[i].TMs)
	}
	g, _ := AgruparAncoras(a, ConfigPadrao())
	for _, x := range g {
		if x.N > 3 {
			t.Fatalf("grupo com %d", x.N)
		}
	}
}

func TestVizinhos(t *testing.T) {
	c := ConfigPadrao()
	p := &ReferenciaVizinho{AMs: 1000, IncertezaBaseMs: 100, TMsPrimeiro: 0, TMsUltimo: 3600000}
	n := &ReferenciaVizinho{AMs: 1000, IncertezaBaseMs: 100, TMsPrimeiro: 20000000, TMsUltimo: 21000000}
	f := FaixaBoot{TMsPrimeiro: 5000000, TMsUltimo: 6000000}
	e := EstimarPorVizinhos(p, n, f, nil, c)
	if e.Origem != "vizinhos" {
		t.Fatalf("%v", e)
	}
	d := 50.0
	e2 := EstimarPorVizinhos(p, nil, f, &d, c)
	if e2.Origem != "" && e2.Origem != "vizinhos" {
		t.Fatal(e2)
	}
	if e3 := EstimarPorVizinhos(nil, n, f, &d, c); e3.Origem != "" {
		t.Fatal("sem anterior")
	}
	if e4 := EstimarPorVizinhos(p, nil, f, nil, c); e4.Origem != "" {
		t.Fatal("sem limite superior")
	}
	baixo := -1e9
	if e5 := EstimarPorVizinhos(p, nil, f, &baixo, c); len(e5.Alertas) != 1 {
		t.Fatal("lo > hi")
	}
}

func TestDesembrulhar(t *testing.T) {
	const v = int64(1) << 32
	if Desembrulhar(5, -1) != 5 {
		t.Fatal()
	}
	if got := Desembrulhar(10, v-100); got != v+10 {
		t.Fatalf("%d", got)
	}
	if got := Desembrulhar(uint32(v-50), v+20); got != v-50 {
		t.Fatalf("%d", got)
	}
	if got := Desembrulhar(100, 50); got != 100 {
		t.Fatalf("%d", got)
	}
}

func TestIncerteza(t *testing.T) {
	g := []Grupo{{Lo: 0, Hi: 400, TCentro: 0, N: 1}, {Lo: 0, Hi: 200, TCentro: 1e7, N: 1}}
	u := Incerteza(1e7+1000, g, ConfigPadrao())
	if math.Abs(u-(100+40e-6*1000)) > 1e-9 {
		t.Fatal(u)
	}
}
