// Package memory implementa os repositórios em memória, só para testes dos casos de uso e do HTTP.
// Não há locking nos repositórios: InTransaction serializa as transações com um mutex e desfaz as
// escritas se fn devolver erro; fora de transação, os testes são sequenciais.
package memory

import (
	"bytes"
	"context"
	"sync"
	"time"

	"tccelta/backend/internal/domain/dongle"
	"tccelta/backend/internal/domain/handshake"
	"tccelta/backend/internal/domain/ingestion"
	"tccelta/backend/internal/domain/occurrence"
	"tccelta/backend/internal/domain/pid"
	"tccelta/backend/internal/domain/protocol"
	"tccelta/backend/internal/domain/timeline"
	"tccelta/backend/internal/port"
)

type challengeKey struct {
	dongle int32
	cs     int64
}

// Challenge é um desafio guardado.
type Challenge struct {
	handshake.StoredChallenge
	Result string
	Final  handshake.Finalization
}

type bootKey struct{ dongle, generation, boot int32 }

// Database é o estado em memória; tudo que é exportado serve para os testes conferirem o resultado.
type Database struct {
	mu sync.Mutex

	dongles    map[protocol.DevID]dongle.Dongle
	states     map[int32]dongle.State
	lastCS     map[int32]int64
	challenges map[challengeKey]Challenge
	tmsBoot    map[bootKey]int64
	seen       map[[2]int64]bool // (dongle, seq) já gravados em raw_block
	Defs       []pid.Def

	RawBlocks     []ingestion.RawBlock
	Rejected      []ingestion.Rejected
	Readings      []pid.Reading
	Positions     []pid.Position
	Gaps          []occurrence.Gap
	Alerts        []occurrence.Alert
	Anchors       []handshake.Anchor
	Notifications []string
	// BootRows é a tabela boot (worker); Groups, os grupos de âncoras por boot; DataBlocks, os blocos de dados por dongle.
	BootRows   []*Boot
	Groups     map[timeline.BootKey][]timeline.Group
	DataBlocks map[int32][]ingestion.StoredBlock
}

// New cria o banco vazio.
func New() *Database {
	return &Database{dongles: map[protocol.DevID]dongle.Dongle{}, states: map[int32]dongle.State{}, lastCS: map[int32]int64{},
		challenges: map[challengeKey]Challenge{}, tmsBoot: map[bootKey]int64{}, seen: map[[2]int64]bool{},
		Groups: map[timeline.BootKey][]timeline.Group{}, DataBlocks: map[int32][]ingestion.StoredBlock{}}
}

// AddDongle cadastra um dongle (e seu estado zerado).
func (b *Database) AddDongle(d dongle.Dongle) {
	b.dongles[d.DevID] = d
	b.states[d.ID] = dongle.State{}
}

// SetState sobrescreve o estado do dongle.
func (b *Database) SetState(id int32, e dongle.State) { b.states[id] = e }

// State devolve o estado atual do dongle.
func (b *Database) State(id int32) dongle.State { return b.states[id] }

// Challenge devolve o desafio (dongle, c_S).
func (b *Database) Challenge(id int32, cs int64) (Challenge, bool) {
	d, ok := b.challenges[challengeKey{id, cs}]
	return d, ok
}

// Repositories devolve os repositórios sobre o banco.
func (b *Database) Repositories() port.Repositories {
	return port.Repositories{Dongles: (*dongles)(b), Challenges: (*challenges)(b), States: (*states)(b),
		Boots: (*boots)(b), Blocks: (*blocks)(b), Occurrences: (*occurrences)(b), Pids: (*pids)(b), AnchorGroups: (*anchorGroups)(b)}
}

// InTransaction implementa port.Transactor.
func (b *Database) InTransaction(ctx context.Context, fn func(ctx context.Context, r port.Repositories) error) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	saved := b.clone()
	if err := fn(ctx, b.Repositories()); err != nil {
		b.restore(saved)
		return err
	}
	return nil
}

func (b *Database) clone() *Database {
	c := New()
	for k, v := range b.dongles {
		c.dongles[k] = v
	}
	for k, v := range b.states {
		c.states[k] = v
	}
	for k, v := range b.lastCS {
		c.lastCS[k] = v
	}
	for k, v := range b.challenges {
		c.challenges[k] = v
	}
	for k, v := range b.tmsBoot {
		c.tmsBoot[k] = v
	}
	for k, v := range b.seen {
		c.seen[k] = v
	}
	c.RawBlocks, c.Rejected, c.Readings, c.Positions = b.RawBlocks[:len(b.RawBlocks):len(b.RawBlocks)], b.Rejected[:len(b.Rejected):len(b.Rejected)],
		b.Readings[:len(b.Readings):len(b.Readings)], b.Positions[:len(b.Positions):len(b.Positions)]
	c.Gaps, c.Alerts = b.Gaps[:len(b.Gaps):len(b.Gaps)], b.Alerts[:len(b.Alerts):len(b.Alerts)]
	c.Anchors, c.Notifications = b.Anchors[:len(b.Anchors):len(b.Anchors)], b.Notifications[:len(b.Notifications):len(b.Notifications)]
	c.BootRows, c.DataBlocks = b.cloneBootRows(), b.DataBlocks
	for k, v := range b.Groups {
		c.Groups[k] = v[:len(v):len(v)]
	}
	return c
}

func (b *Database) restore(c *Database) {
	b.dongles, b.states, b.lastCS, b.challenges, b.tmsBoot, b.seen = c.dongles, c.states, c.lastCS, c.challenges, c.tmsBoot, c.seen
	b.RawBlocks, b.Rejected, b.Readings, b.Positions = c.RawBlocks, c.Rejected, c.Readings, c.Positions
	b.Gaps, b.Alerts, b.Anchors, b.Notifications = c.Gaps, c.Alerts, c.Anchors, c.Notifications
	b.BootRows, b.Groups, b.DataBlocks = c.BootRows, c.Groups, c.DataBlocks
}

type dongles Database

func (r *dongles) FindByDevID(_ context.Context, dev protocol.DevID) (dongle.Dongle, error) {
	d, ok := r.dongles[dev]
	if !ok {
		return dongle.Dongle{}, dongle.ErrUnknown
	}
	return d, nil
}
func (r *dongles) Lock(context.Context, int32) error { return nil }

type challenges Database

func (r *challenges) Issue(_ context.Context, e handshake.Issuance) (handshake.Challenge, error) {
	pend := 0
	for k, d := range r.challenges {
		if k.dongle == e.DongleID && d.AnsweredAt == nil && d.ExpiresAt.After(e.S1) {
			pend++
		}
	}
	if pend >= e.MaxPending {
		return handshake.Challenge{}, handshake.ErrPendingChallenges
	}
	r.lastCS[e.DongleID]++
	cs := r.lastCS[e.DongleID]
	r.challenges[challengeKey{e.DongleID, cs}] = Challenge{StoredChallenge: handshake.StoredChallenge{Nonce: e.Nonce, S1: e.S1, ExpiresAt: e.ExpiresAt}}
	est := r.states[e.DongleID]
	return handshake.Challenge{CS: cs, ContiguousSeq: est.ContiguousSeq, CConf: est.CConf}, nil
}

func (r *challenges) Find(_ context.Context, id int32, cs int64) (handshake.StoredChallenge, error) {
	d, ok := r.challenges[challengeKey{id, cs}]
	if !ok {
		return handshake.StoredChallenge{}, handshake.ErrInvalidChallenge
	}
	return d.StoredChallenge, nil
}

func (r *challenges) Consume(_ context.Context, c handshake.Consumption) (time.Time, error) {
	k := challengeKey{c.DongleID, c.CS}
	d, ok := r.challenges[k]
	if !ok || !bytes.Equal(d.Nonce, c.Nonce) || d.AnsweredAt != nil || !d.ExpiresAt.After(c.S2) {
		return time.Time{}, handshake.ErrInvalidChallenge
	}
	s2 := c.S2
	d.AnsweredAt = &s2
	r.challenges[k] = d
	return d.S1, nil
}

func (r *challenges) Finalize(_ context.Context, f handshake.Finalization) error {
	k := challengeKey{f.DongleID, f.CS}
	d := r.challenges[k]
	d.Result, d.Final = f.Result, f
	r.challenges[k] = d
	return nil
}

func (r *challenges) MarkInvalidTag(_ context.Context, id int32, cs int64, nonce []byte) error {
	k := challengeKey{id, cs}
	if d, ok := r.challenges[k]; ok && bytes.Equal(d.Nonce, nonce) {
		d.Result = "invalid_tag"
		r.challenges[k] = d
	}
	return nil
}

type states Database

func (r *states) Read(_ context.Context, id int32) (dongle.State, error) {
	e := r.states[id]
	e.LastAck = nil
	return e, nil
}

func (r *states) ReadSnapshot(_ context.Context, id int32) (dongle.State, error) {
	e := r.states[id]
	return dongle.State{ContiguousSeq: e.ContiguousSeq, CConf: e.CConf, LastAck: e.LastAck}, nil
}

func (r *states) ReadLastAck(_ context.Context, id int32) ([]byte, error) {
	return r.states[id].LastAck, nil
}

func (r *states) UpdateHandshake(_ context.Context, a handshake.Update) error {
	e := r.states[a.DongleID]
	e.ContiguousSeq, e.MaxSeenSeq, e.HeadSeq, e.CConf, e.LastAck = a.ContiguousSeq, max(e.MaxSeenSeq, a.ContiguousSeq), a.HeadSeq, a.CConf, a.LastAck
	r.states[a.DongleID] = e
	return nil
}

func (r *states) UpdateIngestion(_ context.Context, a ingestion.StateUpdate) error {
	e := r.states[a.DongleID]
	e.ContiguousSeq, e.MaxSeenSeq, e.LastAck = a.ContiguousSeq, max(e.MaxSeenSeq, a.MaxSeen, a.ContiguousSeq), a.LastAck
	e.BootGeneration, e.LastBootID = a.BootGeneration, a.LastBootID
	r.states[a.DongleID] = e
	return nil
}

type boots Database

func (r *boots) LastTMs(_ context.Context, id int32, bootIDs []int32, gMin, gMax int32) ([]timeline.RefBoot, error) {
	var out []timeline.RefBoot
	for _, bid := range bootIDs {
		for g := gMin; g <= gMax; g++ {
			if t, ok := r.tmsBoot[bootKey{id, g, bid}]; ok {
				out = append(out, timeline.RefBoot{Generation: g, BootID: bid, LastTMs: t})
			}
		}
	}
	return out, nil
}

func (r *boots) EnsurePending(context.Context, int32, int32, int32, time.Time) error { return nil }

func (r *boots) UpdateIngestion(_ context.Context, id int32, _ time.Time, aggs []ingestion.BootAggregate) error {
	for _, a := range aggs {
		k := bootKey{id, a.Generation, a.BootID}
		r.tmsBoot[k] = max(r.tmsBoot[k], a.TEnd)
	}
	return nil
}

func (r *boots) InsertAnchor(_ context.Context, a handshake.Anchor) error {
	r.Anchors = append(r.Anchors, a)
	return nil
}

func (r *boots) NotifyPending(_ context.Context, key string) error {
	r.Notifications = append(r.Notifications, key)
	return nil
}

type blocks Database

func (r *blocks) InsertRawBlocks(_ context.Context, id int32, _ int64, _ time.Time, bs []ingestion.RawBlock) (int, error) {
	n := 0
	for _, x := range bs {
		k := [2]int64{int64(id), x.Seq}
		if r.seen[k] {
			continue
		}
		r.seen[k] = true
		r.RawBlocks = append(r.RawBlocks, x)
		n++
	}
	return n, nil
}

func (r *blocks) RegisterRejected(_ context.Context, x ingestion.Rejected) error {
	r.Rejected = append(r.Rejected, x)
	return nil
}

func (r *blocks) CopyReadings(_ context.Context, _ int32, _ time.Time, ls []pid.Reading) error {
	r.Readings = append(r.Readings, ls...)
	return nil
}

func (r *blocks) CopyPositions(_ context.Context, _ int32, _ time.Time, ps []pid.Position) error {
	r.Positions = append(r.Positions, ps...)
	return nil
}

type occurrences Database

func (r *occurrences) InsertGap(_ context.Context, l occurrence.Gap) error {
	r.Gaps = append(r.Gaps, l)
	return nil
}

func (r *occurrences) InsertAlert(_ context.Context, a occurrence.Alert) error {
	r.Alerts = append(r.Alerts, a)
	return nil
}

type pids Database

func (r *pids) List(context.Context) ([]pid.Def, error) { return r.Defs, nil }
