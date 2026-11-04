package tripanalysis

import (
	"math"
	"sort"
)

// Constantes da detecção de marchas (spec 02, "Marcha lenta, ponto morto e marcha").
const (
	gearMinKmh    = 8.0
	gearMinRPM    = 500.0
	gearBinLn     = 0.005
	gearSigmaBins = 3
	gearTolLn     = 0.06
	gearNearLn    = 0.05
	gearMinSepLn  = 0.09
	gearMinStable = 20
	gearMinDwell  = 1000 // ms
	gearFillMs    = 1500
	shiftMaxGapMs = 4000
)

// detectGears classifica os pontos da janela em ponto morto (0), marcha 1..N ou indefinido (-1) e devolve os
// patamares e as trocas. A numeração vale para a viagem inteira (todas as partes usam os mesmos patamares).
func (a *analysis) detectGears(idleRPM float64) ([]Gear, []GearShift) {
	h := a.halfSpan()
	// 1) ponto morto/embreagem e candidatos a marcha
	a.eachPoint(func(p *part, i int) {
		v, r := p.speed[i], p.rpm[i]
		if isNaN(v) || isNaN(r) || v < gearMinKmh {
			return
		}
		if r <= idleRPM+a.p.IdleMarginRPM {
			p.gear[i] = 0
			return
		}
		if r >= gearMinRPM {
			p.lnk[i] = math.Log(r / v)
		}
	})
	// 2) amostras estáveis, parte a parte (a vizinhança não cruza partes)
	var stable []float64
	for _, p := range a.windowed() {
		for i := max(p.first, h); i <= min(p.last, len(p.t)-1-h); i++ {
			if isNaN(p.lnk[i]) {
				continue
			}
			tol := 0.04 + 1/p.speed[i]
			ok := true
			for j := i - h; j <= i+h && ok; j++ {
				ok = j >= p.first && j <= p.last && !isNaN(p.lnk[j]) && math.Abs(p.lnk[j]-p.lnk[i]) <= tol
			}
			if ok {
				stable = append(stable, p.lnk[i])
			}
		}
	}
	if len(stable) < gearMinStable {
		return []Gear{}, []GearShift{}
	}
	centers := a.gearCenters(stable)
	if len(centers) == 0 {
		return []Gear{}, []GearShift{}
	}
	// 3) atribuição por proximidade, limpeza e preenchimento
	for _, p := range a.windowed() {
		for i := p.first; i <= p.last; i++ {
			if isNaN(p.lnk[i]) {
				continue
			}
			best, bd := -1, math.Inf(1)
			for gi, c := range centers {
				if d := math.Abs(p.lnk[i] - c); d < bd {
					best, bd = gi, d
				}
			}
			if bd <= gearTolLn+1/(2*p.speed[i]) {
				p.gear[i] = best + 1
			}
		}
		a.cleanGears(p)
	}
	return a.gearTable(centers), a.shifts()
}

// gearCenters acha os picos do histograma suavizado de ln k; a marcha 1 é a de maior rpm/(km/h).
func (a *analysis) gearCenters(stable []float64) []float64 {
	lo, hi := stable[0], stable[0]
	for _, x := range stable {
		lo, hi = math.Min(lo, x), math.Max(hi, x)
	}
	lo, hi = lo-0.05, hi+0.05
	nb := int(math.Ceil((hi-lo)/gearBinLn)) + 1
	hist := make([]float64, nb)
	for _, x := range stable {
		hist[int(math.Round((x-lo)/gearBinLn))]++
	}
	sm := make([]float64, nb)
	var peak float64
	for b := range sm {
		var acc float64
		for d := -3 * gearSigmaBins; d <= 3*gearSigmaBins; d++ {
			if k := b + d; k >= 0 && k < nb {
				acc += hist[k] * math.Exp(-float64(d*d)/(2*gearSigmaBins*gearSigmaBins))
			}
		}
		sm[b] = acc
		peak = math.Max(peak, acc)
	}
	minMass := int(math.Round(1500 / float64(a.step)))
	type cand struct {
		c, h float64
		near []float64
	}
	var cands []cand
	for b := 1; b < nb-1; b++ {
		if sm[b] < sm[b-1] || sm[b] < sm[b+1] || sm[b] < peak*0.002 {
			continue
		}
		c := lo + float64(b)*gearBinLn
		var near []float64
		for _, x := range stable {
			if math.Abs(x-c) <= gearNearLn {
				near = append(near, x)
			}
		}
		if len(near) >= minMass {
			cands = append(cands, cand{c, sm[b], near})
		}
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].h != cands[j].h {
			return cands[i].h > cands[j].h
		}
		return cands[i].c < cands[j].c
	})
	var kept []cand
	for _, cd := range cands {
		ok := true
		for _, k := range kept {
			if math.Abs(k.c-cd.c) < gearMinSepLn {
				ok = false
				break
			}
		}
		if ok {
			kept = append(kept, cd)
		}
	}
	centers := make([]float64, len(kept))
	for i, k := range kept {
		centers[i] = median(k.near)
	}
	sort.Sort(sort.Reverse(sort.Float64Slice(centers)))
	return centers
}

// cleanGears descarta trechos de marcha com menos de 1000 ms e preenche lacunas de até 1500 ms entre trechos da mesma marcha.
func (a *analysis) cleanGears(p *part) {
	type run struct{ v, i, n int }
	runs := func() []run {
		var out []run
		for i := p.first; i <= p.last; i++ {
			if n := len(out); n > 0 && out[n-1].v == p.gear[i] {
				out[n-1].n++
			} else {
				out = append(out, run{p.gear[i], i, 1})
			}
		}
		return out
	}
	minDwell := int(math.Ceil(float64(gearMinDwell) / float64(a.step)))
	for _, r := range runs() {
		if r.v >= 1 && r.n < minDwell {
			for i := r.i; i < r.i+r.n; i++ {
				p.gear[i] = -1
			}
		}
	}
	rs := runs()
	maxFill := int(math.Floor(float64(gearFillMs) / float64(a.step)))
	for k := 1; k < len(rs)-1; k++ {
		r := rs[k]
		if r.v == -1 && r.n <= maxFill && rs[k-1].v >= 1 && rs[k-1].v == rs[k+1].v {
			for i := r.i; i < r.i+r.n; i++ {
				p.gear[i] = rs[k-1].v
			}
		}
	}
}

// gearTable monta os patamares com o tempo em cada marcha e a fração do tempo com marcha detectada.
func (a *analysis) gearTable(centers []float64) []Gear {
	counts := make([]int, len(centers))
	total := 0
	a.eachPoint(func(p *part, i int) {
		if g := p.gear[i]; g >= 1 {
			counts[g-1]++
			total++
		}
	})
	out := make([]Gear, len(centers))
	for i, c := range centers {
		out[i] = Gear{Gear: i + 1, RPMPerKmh: math.Exp(c), TimeS: float64(counts[i]) * a.dt}
		if total > 0 {
			out[i].TimePct = float64(counts[i]) / float64(total) * 100
		}
	}
	return out
}

// shifts acha as trocas: dois trechos consecutivos de marchas diferentes (ignorando indefinido e ponto morto), com até
// 4000 ms entre o fim de um e o início do outro. Não cruza partes.
func (a *analysis) shifts() []GearShift {
	out := []GearShift{}
	for _, p := range a.windowed() {
		type seg struct{ v, i, n int }
		var segs []seg
		for i := p.first; i <= p.last; i++ {
			g := p.gear[i]
			if g < 1 {
				continue
			}
			if n := len(segs); n > 0 && segs[n-1].v == g && segs[n-1].i+segs[n-1].n == i {
				segs[n-1].n++
			} else {
				segs = append(segs, seg{g, i, 1})
			}
		}
		for k := 1; k < len(segs); k++ {
			s0, s1 := segs[k-1], segs[k]
			if s0.v == s1.v {
				continue
			}
			iEnd, iStart := s0.i+s0.n-1, s1.i
			if p.t[iStart]-p.t[iEnd] > shiftMaxGapMs {
				continue
			}
			// a marcha 1 é a de maior rpm/(km/h): passar para um número maior é subir de marcha
			dir := ShiftDown
			if s1.v > s0.v {
				dir = ShiftUp
			}
			out = append(out, GearShift{TMs: p.t[iEnd], Direction: dir, From: s0.v, To: s1.v,
				RPMBefore: p.rpm[iEnd], RPMAfter: p.rpm[iStart], SpeedKmh: p.speed[iEnd]})
		}
	}
	return out
}
