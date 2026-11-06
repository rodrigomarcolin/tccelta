package tripanalysis

import (
	"math"
	"sort"
)

// PIDs usados (spec 02).
const (
	pidLoad    uint8 = 0x04
	pidCoolant uint8 = 0x05
	pidRPM     uint8 = 0x0C
	pidSpeed   uint8 = 0x0D
	pidMAF     uint8 = 0x10
	pidTank    uint8 = 0x2F
	pidPedal   uint8 = 0x49
)

var nan = math.NaN()

func isNaN(x float64) bool { return math.IsNaN(x) }

// series são as leituras de um PID de uma parte: t crescente, sem repetição.
type series struct {
	t []int64
	v []float64
}

// part é uma parte da viagem com a grade reamostrada. NaN marca a ausência de valor.
type part struct {
	in                 BootRef
	raw                map[uint8]series
	positions          []Position
	clock              *Clock
	dataStart, dataEnd int64

	t                                     []int64
	speed, rpm, pedal, load, coolant, maf []float64
	on                                    []bool
	accel                                 []float64
	gear                                  []int // -1 indefinida, 0 ponto morto/embreagem, 1..N
	lnk                                   []float64
	first, last                           int // janela da parte na grade; -1 se não houve motor ligado
	nSamples                              int64
}

// BootRef identifica o boot de uma parte.
type BootRef struct{ Generation, BootID int32 }

// analysis guarda o estado de um cálculo.
type analysis struct {
	in        Input
	p         Params
	parts     []*part // em ordem cronológica
	step      int64
	dt        float64
	intervals map[uint8]float64 // intervalo mediano por PID (ms)
	neverOn   bool
	overlap   bool
}

func cleanSeries(s []Sample) series {
	ss := make([]Sample, 0, len(s))
	for _, x := range s {
		if math.IsNaN(x.Value) || math.IsInf(x.Value, 0) {
			continue
		}
		ss = append(ss, x)
	}
	sort.SliceStable(ss, func(i, j int) bool { return ss[i].TMs < ss[j].TMs })
	out := series{t: make([]int64, 0, len(ss)), v: make([]float64, 0, len(ss))}
	for _, x := range ss {
		if n := len(out.t); n > 0 && out.t[n-1] == x.TMs {
			continue // fica a primeira leitura de cada t_ms
		}
		out.t = append(out.t, x.TMs)
		out.v = append(out.v, x.Value)
	}
	return out
}

// newPart ordena e limpa as leituras; devolve nil se a parte não tem nenhuma.
func newPart(in PartInput) *part {
	p := &part{in: BootRef{in.BootGeneration, in.BootID}, raw: map[uint8]series{}, clock: in.Clock, first: -1, last: -1}
	for _, ps := range in.Readings {
		s := cleanSeries(ps.Samples)
		if len(s.t) == 0 {
			continue
		}
		p.raw[ps.PID] = s
	}
	if len(p.raw) == 0 {
		return nil
	}
	p.positions = append([]Position(nil), in.Positions...)
	sort.SliceStable(p.positions, func(i, j int) bool { return p.positions[i].TMs < p.positions[j].TMs })
	p.refresh()
	return p
}

// refresh recalcula a faixa de dados e a contagem de leituras.
func (p *part) refresh() {
	p.dataStart, p.dataEnd, p.nSamples = math.MaxInt64, math.MinInt64, 0
	for _, s := range p.raw {
		p.dataStart = min(p.dataStart, s.t[0])
		p.dataEnd = max(p.dataEnd, s.t[len(s.t)-1])
		p.nSamples += int64(len(s.t))
	}
}

// trimFrom descarta leituras e posições a partir de t (sobreposição com a parte seguinte).
func (p *part) trimFrom(t int64) {
	for pid, s := range p.raw {
		k := sort.Search(len(s.t), func(i int) bool { return s.t[i] >= t })
		if k == 0 {
			delete(p.raw, pid)
			continue
		}
		p.raw[pid] = series{t: s.t[:k], v: s.v[:k]}
	}
	k := sort.Search(len(p.positions), func(i int) bool { return p.positions[i].TMs >= t })
	p.positions = p.positions[:k]
	if len(p.raw) > 0 {
		p.refresh()
	}
}

// medianGaps é a mediana dos intervalos positivos entre leituras consecutivas, agregados entre as partes.
func medianGaps(ss []series) float64 {
	var gaps []float64
	for _, s := range ss {
		for i := 1; i < len(s.t); i++ {
			gaps = append(gaps, float64(s.t[i]-s.t[i-1]))
		}
	}
	return median(gaps)
}

// prepare monta as partes (ordenadas, sem sobreposição) e a grade.
func prepare(in Input) *analysis {
	a := &analysis{in: in, p: in.Params, intervals: map[uint8]float64{}}
	for _, pi := range in.Parts {
		if p := newPart(pi); p != nil {
			a.parts = append(a.parts, p)
		}
	}
	sort.SliceStable(a.parts, func(i, j int) bool {
		x, y := a.parts[i], a.parts[j]
		if x.dataStart != y.dataStart {
			return x.dataStart < y.dataStart
		}
		if x.in.Generation != y.in.Generation {
			return x.in.Generation < y.in.Generation
		}
		return x.in.BootID < y.in.BootID
	})
	a.resolveOverlaps()
	if len(a.parts) == 0 {
		return a
	}
	for _, pid := range allPIDs {
		var ss []series
		for _, p := range a.parts {
			if s, ok := p.raw[pid]; ok {
				ss = append(ss, s)
			}
		}
		if iv := medianGaps(ss); !isNaN(iv) {
			a.intervals[pid] = iv
		}
	}
	a.step = a.gridStep()
	a.dt = float64(a.step) / 1000
	for _, p := range a.parts {
		a.buildGrid(p)
	}
	a.findWindows()
	return a
}

var allPIDs = []uint8{pidLoad, pidCoolant, pidRPM, pidSpeed, pidMAF, pidTank, pidPedal}

func uncertainty(p *part) int64 {
	if p.clock == nil {
		return 0
	}
	return int64(p.clock.UncertaintyMs)
}

// resolveOverlaps corta a parte anterior no início da seguinte quando as estimativas pontuais se cruzam e marca o aviso
// quando elas se cruzam estendidas pelas incertezas.
func (a *analysis) resolveOverlaps() {
	for k := 0; k+1 < len(a.parts); k++ {
		p, q := a.parts[k], a.parts[k+1]
		if p.dataEnd+uncertainty(p) > q.dataStart-uncertainty(q) {
			a.overlap = true
		}
		if p.dataEnd >= q.dataStart {
			p.trimFrom(q.dataStart)
			if len(p.raw) == 0 {
				a.parts = append(a.parts[:k], a.parts[k+1:]...)
				k--
			}
		}
	}
}

// gridStep é a mediana dos intervalos medianos de rotação, velocidade e pedal, em múltiplos de 50 ms entre 100 e 1000.
func (a *analysis) gridStep() int64 {
	var m []float64
	for _, pid := range []uint8{pidRPM, pidSpeed, pidPedal} {
		if iv, ok := a.intervals[pid]; ok {
			m = append(m, iv)
		}
	}
	if len(m) == 0 {
		return 1000
	}
	s := int64(math.Round(median(m)/50)) * 50
	return min(1000, max(100, s))
}

func (a *analysis) resample(p *part, pid uint8) []float64 {
	out := make([]float64, len(p.t))
	for i := range out {
		out[i] = nan
	}
	s, ok := p.raw[pid]
	if !ok {
		return out
	}
	iv, ok := a.intervals[pid]
	if !ok {
		iv = float64(a.step)
	}
	maxAge := math.Max(3*iv, 2*float64(a.step))
	j := 0
	for i, g := range p.t {
		for j+1 < len(s.t) && s.t[j+1] <= g {
			j++
		}
		if s.t[j] <= g && float64(g-s.t[j]) <= maxAge {
			out[i] = s.v[j]
		}
	}
	return out
}

func (a *analysis) buildGrid(p *part) {
	n := int((p.dataEnd-p.dataStart)/a.step) + 1
	p.t = make([]int64, n)
	for i := range p.t {
		p.t[i] = p.dataStart + int64(i)*a.step
	}
	p.speed, p.rpm, p.pedal = a.resample(p, pidSpeed), a.resample(p, pidRPM), a.resample(p, pidPedal)
	p.load, p.coolant, p.maf = a.resample(p, pidLoad), a.resample(p, pidCoolant), a.resample(p, pidMAF)
	p.on = make([]bool, n)
	for i, r := range p.rpm {
		p.on[i] = !isNaN(r) && r >= a.p.EngineOnRPM
	}
	p.accel = make([]float64, n)
	p.gear = make([]int, n)
	p.lnk = make([]float64, n)
	for i := range p.accel {
		p.accel[i], p.gear[i], p.lnk[i] = nan, -1, nan
	}
	h := a.halfSpan()
	for i := h; i < n-h; i++ {
		if !isNaN(p.speed[i-h]) && !isNaN(p.speed[i+h]) {
			p.accel[i] = (p.speed[i+h] - p.speed[i-h]) / 3.6 / (2 * float64(h) * a.dt)
		}
	}
}

// halfSpan é h = max(1, round(500/passo)): a meia janela de ±0,5 s em pontos da grade.
func (a *analysis) halfSpan() int {
	return max(1, int(math.Round(500/float64(a.step))))
}

// findWindows define a janela de cada parte (do primeiro ao último ponto com motor ligado) e a da viagem.
func (a *analysis) findWindows() {
	found := false
	for _, p := range a.parts {
		for i, on := range p.on {
			if on {
				if p.first < 0 {
					p.first = i
				}
				p.last = i
				found = true
			}
		}
	}
	if found {
		return
	}
	a.neverOn = true
	for _, p := range a.parts {
		p.first, p.last = 0, len(p.t)-1
	}
}

// windowed são as partes com janela, em ordem.
func (a *analysis) windowed() []*part {
	var out []*part
	for _, p := range a.parts {
		if p.first >= 0 {
			out = append(out, p)
		}
	}
	return out
}

// eachPoint percorre os pontos da grade dentro da janela da viagem.
func (a *analysis) eachPoint(f func(p *part, i int)) {
	for _, p := range a.windowed() {
		for i := p.first; i <= p.last; i++ {
			f(p, i)
		}
	}
}

// rawIn devolve as leituras brutas do PID dentro da janela da parte.
func (p *part) rawIn(pid uint8) (t []int64, v []float64) {
	s, ok := p.raw[pid]
	if !ok || p.first < 0 {
		return nil, nil
	}
	lo, hi := p.t[p.first], p.t[p.last]
	i := sort.Search(len(s.t), func(k int) bool { return s.t[k] >= lo })
	j := sort.Search(len(s.t), func(k int) bool { return s.t[k] > hi })
	return s.t[i:j], s.v[i:j]
}

// positionsIn devolve as posições dentro da janela da parte.
func (p *part) positionsIn() []Position {
	if p.first < 0 {
		return nil
	}
	lo, hi := p.t[p.first], p.t[p.last]
	i := sort.Search(len(p.positions), func(k int) bool { return p.positions[k].TMs >= lo })
	j := sort.Search(len(p.positions), func(k int) bool { return p.positions[k].TMs > hi })
	return p.positions[i:j]
}

func (p *part) hasPID(pid uint8) bool { _, ok := p.raw[pid]; return ok }

func (a *analysis) hasPID(pid uint8) bool {
	for _, p := range a.parts {
		if p.hasPID(pid) {
			return true
		}
	}
	return false
}
