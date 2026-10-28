// Package tempo contém o algoritmo puro da âncora de tempo por boot (spec 06).
package tempo

import (
	"fmt"
	"math"
	"sort"
)

// Desembrulhar escolhe t32 + k*2^32 mais próximo da referência (ref < 0 = sem referência).
func Desembrulhar(t32 uint32, ref int64) int64 {
	const volta = int64(1) << 32
	if ref < 0 {
		return int64(t32)
	}
	base := ref &^ (volta - 1)
	melhor := base + int64(t32)
	for _, c := range []int64{base - volta + int64(t32), base + int64(t32), base + volta + int64(t32)} {
		if abs64(c-ref) < abs64(melhor-ref) {
			melhor = c
		}
	}
	if melhor < 0 {
		melhor = int64(t32)
	}
	return melhor
}

func abs64(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

// Ancora é uma medida (t_ms do dongle, S1 e S2 do servidor em ms Unix).
type Ancora struct {
	TMs        int64
	S1Ms, S2Ms float64
}

// Grupo é um conjunto de âncoras próximas com interseção não vazia.
type Grupo struct {
	Lo, Hi             float64
	TCentro            float64
	TPrimeiro, TUltimo int64
	N                  int
}

func (g Grupo) Centro() float64      { return (g.Lo + g.Hi) / 2 }
func (g Grupo) MeiaLargura() float64 { return (g.Hi - g.Lo) / 2 }

// Alerta é um alerta operacional gerado pelo algoritmo.
type Alerta struct {
	Tipo    string
	Detalhe map[string]any
}

// Estimativa é o resultado: hora_real(t) = AMs + (1+B)*t.
type Estimativa struct {
	AMs             float64
	B               float64
	IncertezaBaseMs int
	Origem          string // "ancoras" | "vizinhos" | ""
	Grupos          []Grupo
	Alertas         []Alerta
}

// Config são os parâmetros do algoritmo.
type Config struct {
	JanelaGrupoMs    float64
	DerivaMaxPPM     float64
	MeiaLarguraMinMs float64
	SpanMinMs        float64
}

// ConfigPadrao: 10 min, 40 ppm, 0,5 ms, 1 h.
func ConfigPadrao() Config {
	return Config{JanelaGrupoMs: 600000, DerivaMaxPPM: 40, MeiaLarguraMinMs: 0.5, SpanMinMs: 3600000}
}

// AgruparAncoras forma os grupos (janela contada da primeira âncora do grupo).
func AgruparAncoras(a []Ancora, c Config) ([]Grupo, []Alerta) {
	if len(a) == 0 {
		return nil, nil
	}
	a = append([]Ancora(nil), a...)
	sort.Slice(a, func(i, j int) bool { return a[i].TMs < a[j].TMs })
	var grupos []Grupo
	var alertas []Alerta
	var soma float64
	var cur Grupo
	abrir := func(x Ancora) {
		cur = Grupo{Lo: x.S1Ms - float64(x.TMs), Hi: x.S2Ms - float64(x.TMs), TPrimeiro: x.TMs, TUltimo: x.TMs, N: 1}
		soma = float64(x.TMs)
	}
	fechar := func() {
		cur.TCentro = soma / float64(cur.N)
		grupos = append(grupos, cur)
	}
	abrir(a[0])
	for _, x := range a[1:] {
		lo, hi := x.S1Ms-float64(x.TMs), x.S2Ms-float64(x.TMs)
		if float64(x.TMs-cur.TPrimeiro) >= c.JanelaGrupoMs {
			fechar()
			abrir(x)
			continue
		}
		nlo, nhi := math.Max(cur.Lo, lo), math.Min(cur.Hi, hi)
		if nlo > nhi {
			alertas = append(alertas, Alerta{"intersecao_vazia", map[string]any{
				"t_ms_a": cur.TUltimo, "t_ms_b": x.TMs, "diferenca_ms": nlo - nhi}})
			fechar()
			abrir(x)
			continue
		}
		cur.Lo, cur.Hi = nlo, nhi
		cur.TUltimo = x.TMs
		cur.N++
		soma += float64(x.TMs)
	}
	fechar()
	return grupos, alertas
}

// AjustarReta devolve (a, b, sigmaB, usouReta). Com um grupo, ou grupos próximos/ruidosos, b = 0.
func ajustar(g []Grupo, c Config) (a, b, sigma float64) {
	if len(g) == 1 {
		return g[0].Centro(), 0, 0
	}
	w := func(x Grupo) float64 { h := math.Max(x.MeiaLargura(), c.MeiaLarguraMinMs); return 1 / (h * h) }
	var S, Sx, Sy float64
	for _, x := range g {
		S += w(x)
		Sx += w(x) * x.TCentro
		Sy += w(x) * x.Centro()
	}
	xm, y0 := Sx/S, g[0].Centro()
	var Sxx, Sxy float64
	for _, x := range g {
		dx, dy := x.TCentro-xm, x.Centro()-y0
		Sxx += w(x) * dx * dx
		Sxy += w(x) * dx * dy
	}
	mediaY := Sy / S
	if Sxx < 1e-9 {
		return mediaY, 0, 0
	}
	b = Sxy / Sxx
	sigma = math.Sqrt(1 / Sxx)
	span := g[len(g)-1].TCentro - g[0].TCentro
	if span < c.SpanMinMs || math.Abs(b) <= sigma {
		return mediaY, 0, sigma
	}
	// o(t) = y0' + b (t - xm) ; a = mediaY - b*xm
	return mediaY - b*xm, b, sigma
}

// AjustarReta expõe (a, b) do ajuste.
func AjustarReta(g []Grupo, c Config) (a, b float64) {
	a, b, _ = ajustar(g, c)
	return
}

// EstimarPorAncoras agrupa, ajusta a reta e gera alertas.
func EstimarPorAncoras(an []Ancora, c Config) Estimativa {
	g, al := AgruparAncoras(an, c)
	if len(g) == 0 {
		return Estimativa{}
	}
	a, b, sigma := ajustar(g, c)
	minMeia := math.Inf(1)
	for _, x := range g {
		minMeia = math.Min(minMeia, x.MeiaLargura())
	}
	if math.Abs(b)-2*sigma > c.DerivaMaxPPM*1e-6 {
		al = append(al, Alerta{"deriva_excessiva", map[string]any{"b_ppm": b * 1e6, "sigma_b_ppm": sigma * 1e6}})
	}
	return Estimativa{AMs: a, B: b, IncertezaBaseMs: int(math.Ceil(minMeia)), Origem: "ancoras", Grupos: g, Alertas: al}
}

// ReferenciaVizinho descreve um boot ancorado vizinho.
type ReferenciaVizinho struct {
	AMs, B                 float64
	IncertezaBaseMs        int
	TMsPrimeiro, TMsUltimo int64
}

// FaixaBoot é a faixa de t_ms do boot sem âncora.
type FaixaBoot struct{ TMsPrimeiro, TMsUltimo int64 }

// EstimarPorVizinhos usa os boots ancorados mais próximos e o limite pelo recebimento.
func EstimarPorVizinhos(ant, prox *ReferenciaVizinho, f FaixaBoot, deslocMaxMs *float64, c Config) Estimativa {
	if ant == nil {
		return Estimativa{}
	}
	ppm := c.DerivaMaxPPM * 1e-6
	tFim := ant.AMs + (1+ant.B)*float64(ant.TMsUltimo)
	uP := float64(ant.IncertezaBaseMs) + ppm*float64(ant.TMsUltimo-ant.TMsPrimeiro)
	lo := (tFim - uP) - float64(f.TMsPrimeiro)
	hi := math.Inf(1)
	if prox != nil {
		tIni := prox.AMs + (1+prox.B)*float64(prox.TMsPrimeiro)
		uN := float64(prox.IncertezaBaseMs) + ppm*float64(prox.TMsUltimo-prox.TMsPrimeiro)
		hi = (tIni + uN) - float64(f.TMsUltimo)
	}
	if deslocMaxMs != nil {
		hi = math.Min(hi, *deslocMaxMs)
	}
	if math.IsInf(hi, 1) {
		return Estimativa{}
	}
	if lo > hi {
		return Estimativa{Alertas: []Alerta{{"intersecao_vazia", map[string]any{"lo_ms": lo, "hi_ms": hi, "origem": "vizinhos"}}}}
	}
	return Estimativa{AMs: (lo + hi) / 2, B: 0, IncertezaBaseMs: int(math.Ceil((hi - lo) / 2)), Origem: "vizinhos"}
}

// Incerteza de uma leitura: meia largura do grupo mais próximo + deriva máxima * distância ao centro.
func Incerteza(t int64, grupos []Grupo, c Config) float64 {
	if len(grupos) == 0 {
		return math.Inf(1)
	}
	melhor := grupos[0]
	for _, g := range grupos[1:] {
		if math.Abs(float64(t)-g.TCentro) < math.Abs(float64(t)-melhor.TCentro) {
			melhor = g
		}
	}
	return melhor.MeiaLargura() + c.DerivaMaxPPM*1e-6*math.Abs(float64(t)-melhor.TCentro)
}

func (e Estimativa) String() string {
	return fmt.Sprintf("{%s a=%.1f b=%.2fppm u=%dms grupos=%d}", e.Origem, e.AMs, e.B*1e6, e.IncertezaBaseMs, len(e.Grupos))
}

// RefBoot é a referência de t_ms (o último t_ms conhecido) de um boot.
type RefBoot struct {
	Geracao, BootID int32
	TMsUltimo       int64
}
