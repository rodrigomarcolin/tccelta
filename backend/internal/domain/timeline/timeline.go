// Package timeline contém o algoritmo puro da âncora de tempo por boot (spec 06).
package timeline

import (
	"fmt"
	"math"
	"sort"
)

// Unwrap escolhe t32 + k*2^32 mais próximo da referência (ref < 0 = sem referência).
func Unwrap(t32 uint32, ref int64) int64 {
	const back = int64(1) << 32
	if ref < 0 {
		return int64(t32)
	}
	base := ref &^ (back - 1)
	best := base + int64(t32)
	for _, c := range []int64{base - back + int64(t32), base + int64(t32), base + back + int64(t32)} {
		if abs64(c-ref) < abs64(best-ref) {
			best = c
		}
	}
	if best < 0 {
		best = int64(t32)
	}
	return best
}

func abs64(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

// Anchor é uma medida (t_ms do dongle, S1 e S2 do servidor em ms Unix).
type Anchor struct {
	TMs        int64
	S1Ms, S2Ms float64
}

// Group é um conjunto de âncoras próximas com interseção não vazia.
type Group struct {
	Lo, Hi        float64
	TCenter       float64
	FirstT, LastT int64
	N             int
}

func (g Group) Center() float64    { return (g.Lo + g.Hi) / 2 }
func (g Group) HalfWidth() float64 { return (g.Hi - g.Lo) / 2 }

// Alert é um alerta operacional gerado pelo algoritmo.
type Alert struct {
	Type   string
	Detail map[string]any
}

// Estimate é o resultado: hora_real(t) = AMs + (1+B)*t.
type Estimate struct {
	AMs               float64
	B                 float64
	BaseUncertaintyMs int
	Source            string // "anchors" | "neighbors" | ""
	Groups            []Group
	Alerts            []Alert
}

// Config são os parâmetros do algoritmo.
type Config struct {
	GroupWindowMs  float64
	MaxDriftPPM    float64
	MinHalfWidthMs float64
	SpanMinMs      float64
}

// DefaultConfig: 10 min, 40 ppm, 0,5 ms, 1 h.
func DefaultConfig() Config {
	return Config{GroupWindowMs: 600000, MaxDriftPPM: 40, MinHalfWidthMs: 0.5, SpanMinMs: 3600000}
}

// GroupAnchors forma os grupos (janela contada da primeira âncora do grupo).
func GroupAnchors(a []Anchor, c Config) ([]Group, []Alert) {
	if len(a) == 0 {
		return nil, nil
	}
	a = append([]Anchor(nil), a...)
	sort.Slice(a, func(i, j int) bool { return a[i].TMs < a[j].TMs })
	var groups []Group
	var alerts []Alert
	var sum float64
	var cur Group
	open := func(x Anchor) {
		cur = Group{Lo: x.S1Ms - float64(x.TMs), Hi: x.S2Ms - float64(x.TMs), FirstT: x.TMs, LastT: x.TMs, N: 1}
		sum = float64(x.TMs)
	}
	close := func() {
		cur.TCenter = sum / float64(cur.N)
		groups = append(groups, cur)
	}
	open(a[0])
	for _, x := range a[1:] {
		lo, hi := x.S1Ms-float64(x.TMs), x.S2Ms-float64(x.TMs)
		if float64(x.TMs-cur.FirstT) >= c.GroupWindowMs {
			close()
			open(x)
			continue
		}
		nlo, nhi := math.Max(cur.Lo, lo), math.Min(cur.Hi, hi)
		if nlo > nhi {
			alerts = append(alerts, Alert{"empty_intersection", map[string]any{
				"t_ms_a": cur.LastT, "t_ms_b": x.TMs, "difference_ms": nlo - nhi}})
			close()
			open(x)
			continue
		}
		cur.Lo, cur.Hi = nlo, nhi
		cur.LastT = x.TMs
		cur.N++
		sum += float64(x.TMs)
	}
	close()
	return groups, alerts
}

// FitLine devolve (a, b, sigmaB, usouReta). Com um grupo, ou grupos próximos/ruidosos, b = 0.
func adjust(g []Group, c Config) (a, b, sigma float64) {
	if len(g) == 1 {
		return g[0].Center(), 0, 0
	}
	w := func(x Group) float64 { h := math.Max(x.HalfWidth(), c.MinHalfWidthMs); return 1 / (h * h) }
	var S, Sx, Sy float64
	for _, x := range g {
		S += w(x)
		Sx += w(x) * x.TCenter
		Sy += w(x) * x.Center()
	}
	xm, y0 := Sx/S, g[0].Center()
	var Sxx, Sxy float64
	for _, x := range g {
		dx, dy := x.TCenter-xm, x.Center()-y0
		Sxx += w(x) * dx * dx
		Sxy += w(x) * dx * dy
	}
	meanY := Sy / S
	if Sxx < 1e-9 {
		return meanY, 0, 0
	}
	b = Sxy / Sxx
	sigma = math.Sqrt(1 / Sxx)
	span := g[len(g)-1].TCenter - g[0].TCenter
	if span < c.SpanMinMs || math.Abs(b) <= sigma {
		return meanY, 0, sigma
	}
	// o(t) = y0' + b (t - xm) ; a = meanY - b*xm
	return meanY - b*xm, b, sigma
}

// FitLine expõe (a, b) do ajuste.
func FitLine(g []Group, c Config) (a, b float64) {
	a, b, _ = adjust(g, c)
	return
}

// EstimateFromAnchors agrupa, ajusta a reta e gera alertas.
func EstimateFromAnchors(an []Anchor, c Config) Estimate {
	g, al := GroupAnchors(an, c)
	if len(g) == 0 {
		return Estimate{}
	}
	a, b, sigma := adjust(g, c)
	minHalf := math.Inf(1)
	for _, x := range g {
		minHalf = math.Min(minHalf, x.HalfWidth())
	}
	if math.Abs(b)-2*sigma > c.MaxDriftPPM*1e-6 {
		al = append(al, Alert{"excessive_drift", map[string]any{"b_ppm": b * 1e6, "sigma_b_ppm": sigma * 1e6}})
	}
	return Estimate{AMs: a, B: b, BaseUncertaintyMs: int(math.Ceil(minHalf)), Source: "anchors", Groups: g, Alerts: al}
}

// NeighborReference descreve um boot ancorado vizinho.
type NeighborReference struct {
	AMs, B            float64
	BaseUncertaintyMs int
	FirstTMs, LastTMs int64
}

// BootRange é a faixa de t_ms do boot sem âncora.
type BootRange struct{ FirstTMs, LastTMs int64 }

// EstimateFromNeighbors usa os boots ancorados mais próximos e o limite pelo recebimento.
func EstimateFromNeighbors(prev, next *NeighborReference, f BootRange, maxOffsetMs *float64, c Config) Estimate {
	if prev == nil {
		return Estimate{}
	}
	ppm := c.MaxDriftPPM * 1e-6
	tEnd := prev.AMs + (1+prev.B)*float64(prev.LastTMs)
	uP := float64(prev.BaseUncertaintyMs) + ppm*float64(prev.LastTMs-prev.FirstTMs)
	lo := (tEnd - uP) - float64(f.FirstTMs)
	hi := math.Inf(1)
	if next != nil {
		tStart := next.AMs + (1+next.B)*float64(next.FirstTMs)
		uN := float64(next.BaseUncertaintyMs) + ppm*float64(next.LastTMs-next.FirstTMs)
		hi = (tStart + uN) - float64(f.LastTMs)
	}
	if maxOffsetMs != nil {
		hi = math.Min(hi, *maxOffsetMs)
	}
	if math.IsInf(hi, 1) {
		return Estimate{}
	}
	if lo > hi {
		return Estimate{Alerts: []Alert{{"empty_intersection", map[string]any{"lo_ms": lo, "hi_ms": hi, "source": "neighbors"}}}}
	}
	return Estimate{AMs: (lo + hi) / 2, B: 0, BaseUncertaintyMs: int(math.Ceil((hi - lo) / 2)), Source: "neighbors"}
}

// Uncertainty de uma leitura: meia largura do grupo mais próximo + deriva máxima * distância ao centro.
func Uncertainty(t int64, groups []Group, c Config) float64 {
	if len(groups) == 0 {
		return math.Inf(1)
	}
	best := groups[0]
	for _, g := range groups[1:] {
		if math.Abs(float64(t)-g.TCenter) < math.Abs(float64(t)-best.TCenter) {
			best = g
		}
	}
	return best.HalfWidth() + c.MaxDriftPPM*1e-6*math.Abs(float64(t)-best.TCenter)
}

func (e Estimate) String() string {
	return fmt.Sprintf("{%s a=%.1f b=%.2fppm u=%dms grupos=%d}", e.Source, e.AMs, e.B*1e6, e.BaseUncertaintyMs, len(e.Groups))
}

// RefBoot é a referência de t_ms (o último t_ms conhecido) de um boot.
type RefBoot struct {
	Generation, BootID int32
	LastTMs            int64
}
