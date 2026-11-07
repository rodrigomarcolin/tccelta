package memory

import (
	"context"
	"encoding/json"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"tccelta/backend/internal/domain/dongle"
	"tccelta/backend/internal/domain/timeline"
	"tccelta/backend/internal/domain/trip"
	"tccelta/backend/internal/domain/tripanalysis"
)

// TripRow é uma linha de gold.trip com o resultado gravado pela última Save (nil até a viagem ficar pronta).
type TripRow struct {
	trip.Trip
	Calc *trip.Calculation
	// ExcludedAt é o excluded_at de gold.trip: a linha segue no banco, mas some de tudo (EXC1).
	ExcludedAt *time.Time
	// Saves conta as gravações do resultado.
	Saves int
}

// SourceBlock é um bloco de dados de bronze.raw_block, só com o que o cálculo da viagem lê.
type SourceBlock struct {
	DongleID           int32
	Seq                int64
	Generation, BootID int32
	T0Ms               int64
	ReceivedAt         time.Time
}

// SourceReading e SourcePosition são linhas da silver (reading e position) lidas pelo cálculo.
type (
	SourceReading struct {
		DongleID           int32
		Generation, BootID int32
		PID                int16
		TMs                int64
		Value              float64
		ReceivedAt         time.Time
	}
	SourcePosition struct {
		DongleID           int32
		Generation, BootID int32
		TMs                int64
		Lat, Lon           float64
		ReceivedAt         time.Time
	}
)

// AddTrip insere uma viagem pending recém-criada (a API2 faz isso no banco real) e a devolve para os testes alterarem.
func (b *Database) AddTrip(t trip.Trip) *TripRow {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	if t.Status == "" {
		t.Status = trip.StatusPending
	}
	r := &TripRow{Trip: t}
	b.Trips = append(b.Trips, r)
	return r
}

// TripByID devolve a linha da viagem.
func (b *Database) TripByID(id uuid.UUID) *TripRow {
	for _, x := range b.Trips {
		if x.ID == id {
			return x
		}
	}
	return nil
}

// SetLastAck define o horário do último lote confirmado do dongle.
func (b *Database) SetLastAck(dongleID int32, t time.Time) { b.lastAckAt[dongleID] = t }

func (b *Database) cloneTrips() []*TripRow {
	out := make([]*TripRow, len(b.Trips))
	for i, x := range b.Trips {
		c := *x
		out[i] = &c
	}
	return out
}

func (r *states) ReadProgress(_ context.Context, id int32) (dongle.Progress, error) {
	e := r.states[id]
	p := dongle.Progress{ContiguousSeq: e.ContiguousSeq, HeadSeq: e.HeadSeq}
	if t, ok := r.lastAckAt[id]; ok {
		p.LastAckAt = &t
	}
	return p, nil
}

func (r *anchorGroups) List(_ context.Context, k timeline.BootKey) ([]timeline.Group, error) {
	return append([]timeline.Group(nil), r.Groups[k]...), nil
}

type trips Database

// live diz se a linha existe para o resto do sistema (não foi excluída).
func (x *TripRow) live() bool { return x.ExcludedAt == nil }

func (r *trips) ListWaiting(context.Context) ([]trip.Trip, error) {
	var out []trip.Trip
	for _, x := range r.Trips {
		if x.live() && (x.Status == trip.StatusPending || x.Status == trip.StatusWaitingData) {
			out = append(out, x.Trip)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (r *trips) MarkWaiting(_ context.Context, id uuid.UUID) error {
	if x := (*Database)(r).TripByID(id); x != nil && x.Status == trip.StatusPending {
		x.Status = trip.StatusWaitingData
	}
	return nil
}

func (r *trips) ResolveGeneration(_ context.Context, id uuid.UUID, dongleID, bootID int32) (int32, bool, error) {
	x := (*Database)(r).TripByID(id)
	if x == nil || x.BootGeneration != nil {
		return 0, false, nil
	}
	found, gen := false, int32(0)
	for _, b := range r.BootRows {
		if b.DongleID == dongleID && b.BootID == bootID && (!found || b.Generation > gen) {
			found, gen = true, b.Generation
		}
	}
	if found {
		g := gen
		x.BootGeneration = &g
	}
	return gen, found, nil
}

func (r *trips) Take(_ context.Context, id uuid.UUID) (trip.Trip, bool, error) {
	if x := (*Database)(r).TripByID(id); x != nil && x.live() && (x.Status == trip.StatusPending || x.Status == trip.StatusWaitingData) {
		return x.Trip, true, nil
	}
	return trip.Trip{}, false, nil
}

func (r *trips) TakeRecalc(context.Context) (trip.Trip, bool, error) {
	var best *TripRow
	for _, x := range r.Trips {
		if x.live() && x.Status == trip.StatusReady && x.RecalcPending && (best == nil || x.CreatedAt.Before(best.CreatedAt)) {
			best = x
		}
	}
	if best == nil {
		return trip.Trip{}, false, nil
	}
	return best.Trip, true, nil
}

func (r *trips) Save(_ context.Context, id uuid.UUID, c trip.Calculation) error {
	x := (*Database)(r).TripByID(id)
	if x == nil {
		return nil
	}
	v, at, through := tripanalysis.CalculationVersion, c.CalculatedAt, c.CalculatedThroughSeq
	x.Status, x.CalculationVersion, x.CalculatedAt = trip.StatusReady, &v, &at
	x.FirstSeq, x.LastSeq, x.CalculatedThroughSeq = c.FirstSeq, c.LastSeq, &through
	x.Provisional, x.RecalcPending = c.Provisional, false
	cc := c
	x.Calc = &cc
	x.Saves++
	return nil
}

func (r *trips) Fail(_ context.Context, id uuid.UUID) error {
	if x := (*Database)(r).TripByID(id); x != nil && x.live() {
		if x.Status != trip.StatusReady {
			x.Status = trip.StatusFailed
		}
		x.RecalcPending = false
	}
	return nil
}

func (r *trips) ListProvisional(context.Context) ([]trip.Provisional, error) {
	var out []trip.Provisional
	for _, x := range r.Trips {
		if !x.live() || x.Status != trip.StatusReady || !x.Provisional || x.RecalcPending {
			continue
		}
		p := trip.Provisional{Trip: x.Trip}
		if x.Calc != nil {
			for _, w := range x.Calc.Result.Warnings {
				if w.Code != tripanalysis.WarnBootNoTime {
					continue
				}
				if g, ok := w.Detail["boot_generation"].(int32); ok {
					if id, ok := w.Detail["boot_id"].(int32); ok {
						p.NoTimeBoots = append(p.NoTimeBoots, trip.BootRef{Generation: g, BootID: id})
					}
				}
			}
		}
		out = append(out, p)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (r *trips) MarkRecalc(_ context.Context, ids []uuid.UUID) error {
	for _, id := range ids {
		if x := (*Database)(r).TripByID(id); x != nil && x.Status == trip.StatusReady {
			x.RecalcPending = true
		}
	}
	return nil
}

func (r *trips) ClearProvisional(_ context.Context, id uuid.UUID) error {
	if x := (*Database)(r).TripByID(id); x != nil && x.Status == trip.StatusReady {
		x.Provisional = false
	}
	return nil
}

func (r *trips) MarkRecalcByRange(_ context.Context, dongleID int32, fromSeq, toSeq int64) error {
	for _, x := range r.Trips {
		if x.live() && x.DongleID == dongleID && x.Status == trip.StatusReady && x.FirstSeq != nil && x.LastSeq != nil &&
			*x.FirstSeq <= toSeq && *x.LastSeq >= fromSeq {
			x.RecalcPending = true
		}
	}
	return nil
}

func (r *trips) Notify(context.Context) error {
	r.TripNotifications++
	return nil
}

func (r *trips) CountQueue(context.Context) (trip.Queue, error) {
	var q trip.Queue
	for _, x := range r.Trips {
		if x.live() && (x.Status == trip.StatusPending || x.Status == trip.StatusWaitingData) {
			q.Waiting++
		}
		if x.live() && x.RecalcPending {
			q.Recalc++
		}
	}
	return q, nil
}

type tripSources Database

func (r *tripSources) toBoot(b *Boot) trip.Boot {
	return trip.Boot{Generation: b.Generation, BootID: b.BootID, FirstSeq: b.FirstSeq, LastSeq: b.LastSeq, FirstTMs: b.FirstTMs,
		LastTMs: b.LastTMs, MaxOffsetMs: b.MaxOffsetMs, AMs: b.AMs, B: b.B, BaseUncertaintyMs: b.BaseUncertaintyMs, TimeSource: b.TimeSource}
}

func (r *tripSources) FindBoot(_ context.Context, dongleID, generation, bootID int32) (trip.Boot, bool, error) {
	if b := (*Database)(r).bootRow(timeline.BootKey{DongleID: dongleID, Generation: generation, BootID: bootID}); b != nil {
		return r.toBoot(b), true, nil
	}
	return trip.Boot{}, false, nil
}

func (r *tripSources) sortedBoots(keep func(b *Boot) bool) []trip.Boot {
	var out []trip.Boot
	for _, b := range r.BootRows {
		if keep(b) {
			out = append(out, r.toBoot(b))
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return *out[i].FirstSeq < *out[j].FirstSeq })
	return out
}

func (r *tripSources) IntervalBoots(_ context.Context, dongleID int32, fromMs, toMs float64) ([]trip.Boot, error) {
	return r.sortedBoots(func(b *Boot) bool {
		tb := r.toBoot(b)
		return b.DongleID == dongleID && tb.HasTime() && b.FirstSeq != nil && b.FirstTMs != nil && b.LastTMs != nil &&
			tb.EpochMs(float64(*b.LastTMs)) >= fromMs && tb.EpochMs(float64(*b.FirstTMs)) < toMs
	}), nil
}

func (r *tripSources) UnconvertedBoots(_ context.Context, dongleID int32, t0Ms, t1Ms float64) ([]trip.Boot, error) {
	lo, hi := int64(0), int64(math.MaxInt64)
	for _, b := range r.BootRows {
		tb := r.toBoot(b)
		if b.DongleID != dongleID || !tb.HasTime() || b.FirstSeq == nil || b.FirstTMs == nil || b.LastTMs == nil {
			continue
		}
		if tb.EpochMs(float64(*b.LastTMs)) < t0Ms {
			lo = max(lo, *b.FirstSeq)
		}
		if tb.EpochMs(float64(*b.FirstTMs)) >= t1Ms {
			hi = min(hi, *b.FirstSeq)
		}
	}
	return r.sortedBoots(func(b *Boot) bool {
		return b.DongleID == dongleID && b.TimeSource == nil && b.FirstSeq != nil && b.MaxOffsetMs != nil && b.LastTMs != nil &&
			*b.MaxOffsetMs+float64(*b.LastTMs) >= t0Ms && *b.FirstSeq > lo && *b.FirstSeq < hi
	}), nil
}

func (r *tripSources) ReceivedRange(_ context.Context, dongleID int32, b trip.Boot) (time.Time, time.Time, bool, error) {
	var first, last time.Time
	found := false
	for _, x := range r.SourceBlocks {
		if x.DongleID != dongleID || x.Generation != b.Generation || x.BootID != b.BootID || b.FirstSeq == nil || b.LastSeq == nil ||
			x.Seq < *b.FirstSeq || x.Seq > *b.LastSeq {
			continue
		}
		if !found || x.ReceivedAt.Before(first) {
			first = x.ReceivedAt
		}
		if !found || x.ReceivedAt.After(last) {
			last = x.ReceivedAt
		}
		found = true
	}
	return first, last, found, nil
}

func (r *tripSources) LoadReadings(_ context.Context, q trip.ReadingQuery) ([]trip.ReadingRow, error) {
	want := map[int16]bool{}
	for _, p := range q.PIDs {
		want[p] = true
	}
	var out []trip.ReadingRow
	for _, x := range r.SourceReadings {
		if x.DongleID == q.DongleID && x.Generation == q.Generation && x.BootID == q.BootID && want[x.PID] &&
			!x.ReceivedAt.Before(q.ReceivedFrom) && !x.ReceivedAt.After(q.ReceivedTo) && x.TMs >= q.TFrom && x.TMs <= q.TTo {
			out = append(out, trip.ReadingRow{PID: x.PID, TMs: x.TMs, Value: x.Value})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].PID != out[j].PID {
			return out[i].PID < out[j].PID
		}
		return out[i].TMs < out[j].TMs
	})
	return out, nil
}

func (r *tripSources) LoadPositions(_ context.Context, q trip.ReadingQuery) ([]trip.PositionRow, error) {
	var out []trip.PositionRow
	for _, x := range r.SourcePositions {
		if x.DongleID == q.DongleID && x.Generation == q.Generation && x.BootID == q.BootID &&
			!x.ReceivedAt.Before(q.ReceivedFrom) && !x.ReceivedAt.After(q.ReceivedTo) && x.TMs >= q.TFrom && x.TMs <= q.TTo {
			out = append(out, trip.PositionRow{TMs: x.TMs, Lat: x.Lat, Lon: x.Lon})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].TMs < out[j].TMs })
	return out, nil
}

func (r *tripSources) NextBootStart(_ context.Context, dongleID int32, afterSeq int64) (int64, bool, error) {
	best, found := int64(0), false
	for _, b := range r.BootRows {
		if b.DongleID == dongleID && b.FirstSeq != nil && *b.FirstSeq > afterSeq && (!found || *b.FirstSeq < best) {
			best, found = *b.FirstSeq, true
		}
	}
	return best, found, nil
}

func (r *tripSources) MaxBlockTime(_ context.Context, dongleID int32, fromSeq, toSeq int64) (float64, bool, error) {
	best, found := 0.0, false
	for _, x := range r.SourceBlocks {
		if x.DongleID != dongleID || x.Seq < fromSeq || x.Seq > toSeq {
			continue
		}
		b := (*Database)(r).bootRow(timeline.BootKey{DongleID: dongleID, Generation: x.Generation, BootID: x.BootID})
		if b == nil {
			continue
		}
		if tb := r.toBoot(b); tb.HasTime() {
			if ms := tb.EpochMs(float64(x.T0Ms)); !found || ms > best {
				best, found = ms, true
			}
		}
	}
	return best, found, nil
}

// Insert imita gold.trip: a viagem entra em pending e o índice único (categoria, motorista normalizado) vale para as que não
// falharam. Como no banco, o id e created_at são definidos aqui.
func (r *trips) Insert(_ context.Context, d trip.Draft) (trip.Trip, error) {
	for _, x := range r.Trips {
		if x.live() && sameEntry(x.Trip, d) {
			return trip.Trip{}, trip.ErrTripRepeated
		}
	}
	for _, x := range r.Trips {
		if x.live() && x.Status != trip.StatusFailed && x.Category == d.Category && trip.NormalizeName(x.DriverName) == d.DriverNameNormalized {
			return trip.Trip{}, trip.ErrDriverRepeated
		}
	}
	row := (*Database)(r).AddTrip(trip.Trip{Category: d.Category, DriverName: d.DriverName, Fuel: d.Fuel, DongleID: d.DongleID,
		EntryType: d.EntryType, BootGeneration: d.BootGeneration, BootID: d.BootID, IntervalStart: d.IntervalStart,
		IntervalEnd: d.IntervalEnd, TargetSeq: d.TargetSeq, CreatedAt: time.Now().UTC()})
	return row.Trip, nil
}

func (r *trips) NotifyTrip(_ context.Context, id uuid.UUID) error {
	r.TripNotifications++
	r.TripNotified = append(r.TripNotified, id)
	return nil
}

func (r *trips) Find(_ context.Context, id uuid.UUID) (trip.Trip, error) {
	if x := (*Database)(r).TripByID(id); x != nil && x.live() {
		return x.Trip, nil
	}
	return trip.Trip{}, trip.ErrNotFound
}

// Analysis monta a leitura a partir do resultado guardado por Save.
func (r *trips) Analysis(_ context.Context, id uuid.UUID) (trip.Analysis, error) {
	x := (*Database)(r).TripByID(id)
	if x == nil || !x.live() {
		return trip.Analysis{}, trip.ErrNotFound
	}
	a := trip.Analysis{Trip: x.Trip}
	if x.Status != trip.StatusReady || x.Calc == nil {
		return a, nil
	}
	res := x.Calc.Result
	raw := func(v any) trip.RawJSON {
		b, _ := json.Marshal(v)
		return b
	}
	list := func(v any) trip.RawJSON {
		if b := raw(v); string(b) != "null" {
			return b
		}
		return trip.RawJSON("[]")
	}
	a.Parts, a.Summary, a.SpeedBands, a.Events, a.Stops, a.Gears, a.GearShifts = res.Parts, res.Summary, res.SpeedBands,
		res.Events, res.Stops, res.Gears, res.GearShifts
	a.Warnings, a.Params = list(res.Warnings), raw(res.Params)
	s := res.Series
	a.Series = trip.AnalysisSeries{StepMs: s.StepMs, ExposureStepMs: s.ExposureStepMs, Series: raw(s.Series), Slow: raw(s.Slow),
		RPMDistribution: list(s.RPMDistribution), PedalDistribution: list(s.PedalDistribution), RPMPedalGrid: raw(s.RPMPedal)}
	if len(s.Route.TMs) > 0 {
		a.Series.Route = raw(s.Route)
	}
	return a, nil
}

// sameEntry diz se a viagem x ocupa a mesma chave dos índices trip_boot_unique e trip_interval_unique que d (a geração nula nunca
// colide, como o NULL do banco). Só conta quem não falhou.
func sameEntry(x trip.Trip, d trip.Draft) bool {
	if x.Status == trip.StatusFailed || x.DongleID != d.DongleID || x.EntryType != d.EntryType {
		return false
	}
	if d.EntryType == trip.EntryTypeBoot {
		return x.BootGeneration != nil && d.BootGeneration != nil && *x.BootGeneration == *d.BootGeneration &&
			x.BootID != nil && d.BootID != nil && *x.BootID == *d.BootID
	}
	return x.IntervalStart != nil && d.IntervalStart != nil && x.IntervalStart.Equal(*d.IntervalStart) &&
		x.IntervalEnd != nil && d.IntervalEnd != nil && x.IntervalEnd.Equal(*d.IntervalEnd)
}

func (r *trips) FindSame(_ context.Context, d trip.Draft) (trip.Trip, bool, error) {
	var best *TripRow
	for _, x := range r.Trips {
		if !x.live() || !sameEntry(x.Trip, d) {
			continue
		}
		if best == nil || (x.Status == trip.StatusReady && best.Status != trip.StatusReady) ||
			((x.Status == trip.StatusReady) == (best.Status == trip.StatusReady) && x.CreatedAt.Before(best.CreatedAt)) {
			best = x
		}
	}
	if best == nil {
		return trip.Trip{}, false, nil
	}
	return best.Trip, true, nil
}

// List imita a consulta do banco: da mais nova para a mais antiga, desempatando pelo id, com o cursor e a busca por trecho do nome.
func (r *trips) List(_ context.Context, q trip.ListQuery) (trip.Page, error) {
	var rows []*TripRow
	for _, x := range r.Trips {
		if x.live() && (q.Search == "" || strings.Contains(trip.NormalizeName(x.DriverName), q.Search)) {
			rows = append(rows, x)
		}
	}
	total := int64(len(rows))
	sort.SliceStable(rows, func(i, j int) bool {
		if !rows[i].CreatedAt.Equal(rows[j].CreatedAt) {
			return rows[i].CreatedAt.After(rows[j].CreatedAt)
		}
		return rows[i].ID.String() > rows[j].ID.String()
	})
	var items []trip.ListItem
	for _, x := range rows {
		if c := q.Cursor; c != nil && (x.CreatedAt.After(c.CreatedAt) || (x.CreatedAt.Equal(c.CreatedAt) && x.ID.String() >= c.ID.String())) {
			continue
		}
		it := trip.ListItem{Trip: x.Trip}
		if x.Status == trip.StatusReady && x.Calc != nil {
			s := x.Calc.Result.Summary
			it.StartedAt, it.EndedAt, it.DistanceKm, it.DurationS = s.StartedAt, s.EndedAt, s.DistanceKm, s.DurationS
			it.Route = trip.RoutePreview(x.Calc.Result.Series.Route)
		}
		items = append(items, it)
		if len(items) == q.Limit+1 {
			break
		}
	}
	return trip.PageOf(items, total, q.Limit), nil
}

// Exclude imita ExcludeTrip: trip.ErrNotFound se a viagem não existe ou já foi excluída.
func (r *trips) Exclude(_ context.Context, id uuid.UUID) error {
	x := (*Database)(r).TripByID(id)
	if x == nil || !x.live() {
		return trip.ErrNotFound
	}
	now := time.Now().UTC()
	x.ExcludedAt = &now
	return nil
}
