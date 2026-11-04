package tripcalc

import (
	"context"
	"math"
	"sort"

	"tccelta/backend/internal/domain/timeline"
	"tccelta/backend/internal/domain/trip"
	"tccelta/backend/internal/domain/tripanalysis"
	"tccelta/backend/internal/port"
)

// folga é a margem, nos dois lados do intervalo, ao escolher os boots e as linhas candidatas (spec API1).
const folgaMs = 5000

// pids são os PIDs que a análise usa (spec 02).
var pids = []int16{0x04, 0x05, 0x0C, 0x0D, 0x10, 0x2F, 0x49}

// loaded é o que o carregador devolve para o cálculo.
type loaded struct {
	parts    []tripanalysis.PartInput
	warnings []tripanalysis.Warning
	// boots são os boots das partes, em ordem de first_seq.
	boots             []trip.Boot
	firstSeq, lastSeq *int64
}

// load carrega as partes da viagem no eixo da análise (spec API1, "Conversão de hora e seleção dos dados").
func (c *Calculator) load(ctx context.Context, r port.Repositories, t trip.Trip) (loaded, error) {
	if t.EntryType == "interval" {
		return c.loadInterval(ctx, r, t)
	}
	return c.loadBoot(ctx, r, t)
}

func (c *Calculator) loadBoot(ctx context.Context, r port.Repositories, t trip.Trip) (loaded, error) {
	var l loaded
	if t.BootGeneration == nil || t.BootID == nil {
		return l, nil // o boot ainda não chegou ao servidor
	}
	b, found, err := r.TripSources.FindBoot(ctx, t.DongleID, *t.BootGeneration, *t.BootID)
	if err != nil || !found {
		return l, err
	}
	rows, pos, err := c.fetch(ctx, r, t.DongleID, b, math.MinInt64, math.MaxInt64)
	if err != nil {
		return l, err
	}
	if len(rows) == 0 && len(pos) == 0 {
		return l, nil
	}
	p := tripanalysis.PartInput{BootGeneration: b.Generation, BootID: b.BootID, Readings: groupReadings(rows)}
	for _, x := range pos {
		p.Positions = append(p.Positions, tripanalysis.Position{TMs: x.TMs, Lat: x.Lat, Lon: x.Lon})
	}
	if b.HasTime() {
		lo, hi := extent(rows, pos)
		u, err := c.uncertainty(ctx, r, t.DongleID, b, lo, hi)
		if err != nil {
			return l, err
		}
		p.Clock = &tripanalysis.Clock{AMs: *b.AMs, B: *b.B, UncertaintyMs: u}
	}
	l.parts = []tripanalysis.PartInput{p}
	l.boots = []trip.Boot{b}
	l.firstSeq, l.lastSeq = b.FirstSeq, b.LastSeq
	return l, nil
}

func (c *Calculator) loadInterval(ctx context.Context, r port.Repositories, t trip.Trip) (loaded, error) {
	var l loaded
	t0, t1 := float64(t.IntervalStart.UnixMilli()), float64(t.IntervalEnd.UnixMilli())
	span := int64(t1 - t0)
	boots, err := r.TripSources.IntervalBoots(ctx, t.DongleID, t0-folgaMs, t1+folgaMs)
	if err != nil {
		return l, err
	}
	for _, b := range boots {
		if !b.HasTime() {
			continue
		}
		// inversa da reta, com a mesma folga, para o filtro de t_ms no banco
		tFrom := int64(math.Floor((t0 - folgaMs - *b.AMs) / (1 + *b.B)))
		tTo := int64(math.Ceil((t1 + folgaMs - *b.AMs) / (1 + *b.B)))
		rows, pos, err := c.fetch(ctx, r, t.DongleID, b, tFrom, tTo)
		if err != nil {
			return l, err
		}
		axis := func(tMs int64) (int64, bool) {
			a := int64(math.Round(b.EpochMs(float64(tMs)) - t0))
			return a, a >= 0 && a < span
		}
		// keptRows/keptPos estão no eixo da análise; origRows/origPos guardam o t_ms do boot, que é a escala das âncoras
		var keptRows, origRows []trip.ReadingRow
		for _, x := range rows {
			if a, ok := axis(x.TMs); ok {
				keptRows = append(keptRows, trip.ReadingRow{PID: x.PID, TMs: a, Value: x.Value})
				origRows = append(origRows, x)
			}
		}
		var keptPos, origPos []trip.PositionRow
		for _, x := range pos {
			if a, ok := axis(x.TMs); ok {
				keptPos = append(keptPos, trip.PositionRow{TMs: a, Lat: x.Lat, Lon: x.Lon})
				origPos = append(origPos, x)
			}
		}
		if len(keptRows) == 0 && len(keptPos) == 0 {
			continue
		}
		lo, hi := extent(origRows, origPos)
		u, err := c.uncertainty(ctx, r, t.DongleID, b, lo, hi)
		if err != nil {
			return l, err
		}
		p := tripanalysis.PartInput{BootGeneration: b.Generation, BootID: b.BootID, Readings: groupReadings(keptRows),
			Clock: &tripanalysis.Clock{AMs: *b.AMs, B: *b.B, UncertaintyMs: u}}
		for _, x := range keptPos {
			p.Positions = append(p.Positions, tripanalysis.Position{TMs: x.TMs, Lat: x.Lat, Lon: x.Lon})
		}
		l.parts = append(l.parts, p)
		l.boots = append(l.boots, b)
		if b.FirstSeq != nil && (l.firstSeq == nil || *b.FirstSeq < *l.firstSeq) {
			l.firstSeq = b.FirstSeq
		}
		if b.LastSeq != nil && (l.lastSeq == nil || *b.LastSeq > *l.lastSeq) {
			l.lastSeq = b.LastSeq
		}
	}
	unconverted, err := r.TripSources.UnconvertedBoots(ctx, t.DongleID, t0, t1)
	if err != nil {
		return l, err
	}
	for _, b := range unconverted {
		l.warnings = append(l.warnings, tripanalysis.Warning{Code: tripanalysis.WarnBootNoTime,
			Detail: map[string]any{"boot_generation": b.Generation, "boot_id": b.BootID}})
	}
	return l, nil
}

// fetch lê as leituras e posições do boot dentro de [tFrom, tTo], com o intervalo de received_at dos blocos do boot
// (a hypertable é particionada por received_at).
func (c *Calculator) fetch(ctx context.Context, r port.Repositories, dongleID int32, b trip.Boot, tFrom, tTo int64) ([]trip.ReadingRow, []trip.PositionRow, error) {
	from, to, ok, err := r.TripSources.ReceivedRange(ctx, dongleID, b)
	if err != nil || !ok {
		return nil, nil, err
	}
	q := trip.ReadingQuery{DongleID: dongleID, Generation: b.Generation, BootID: b.BootID, PIDs: pids,
		ReceivedFrom: from, ReceivedTo: to, TFrom: tFrom, TTo: tTo}
	rows, err := r.TripSources.LoadReadings(ctx, q)
	if err != nil {
		return nil, nil, err
	}
	pos, err := r.TripSources.LoadPositions(ctx, q)
	return rows, pos, err
}

// groupReadings agrupa as linhas (ordenadas por pid e t_ms) em uma série por PID, em ordem de PID.
func groupReadings(rows []trip.ReadingRow) []tripanalysis.PIDSeries {
	var out []tripanalysis.PIDSeries
	for _, x := range rows {
		if n := len(out); n == 0 || out[n-1].PID != uint8(x.PID) {
			out = append(out, tripanalysis.PIDSeries{PID: uint8(x.PID)})
		}
		s := &out[len(out)-1]
		s.Samples = append(s.Samples, tripanalysis.Sample{TMs: x.TMs, Value: x.Value})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].PID < out[j].PID })
	return out
}

// extent devolve o menor e o maior t_ms das linhas.
func extent(rows []trip.ReadingRow, pos []trip.PositionRow) (lo, hi int64) {
	lo, hi = math.MaxInt64, math.MinInt64
	for _, x := range rows {
		lo, hi = min(lo, x.TMs), max(hi, x.TMs)
	}
	for _, x := range pos {
		lo, hi = min(lo, x.TMs), max(hi, x.TMs)
	}
	return lo, hi
}

// uncertainty é a incerteza da conversão de hora da parte: o maior valor entre o primeiro e o último dado (a função é em V).
// Boot sem grupos (hora pelos vizinhos) usa a incerteza base.
func (c *Calculator) uncertainty(ctx context.Context, r port.Repositories, dongleID int32, b trip.Boot, first, last int64) (int32, error) {
	base := float64(0)
	if b.BaseUncertaintyMs != nil {
		base = float64(*b.BaseUncertaintyMs)
	}
	groups, err := r.AnchorGroups.List(ctx, timeline.BootKey{DongleID: dongleID, Generation: b.Generation, BootID: b.BootID})
	if err != nil {
		return 0, err
	}
	if len(groups) == 0 || b.TimeSource == nil || *b.TimeSource != "anchors" || first > last {
		return int32(math.Ceil(base)), nil
	}
	u := math.Max(timeline.Uncertainty(first, groups, c.cfg.Timeline), timeline.Uncertainty(last, groups, c.cfg.Timeline))
	return int32(math.Ceil(u)), nil
}
