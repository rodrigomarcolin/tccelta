package boottime

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"tccelta/backend/internal/adapter/memory"
	"tccelta/backend/internal/domain/dongle"
	"tccelta/backend/internal/domain/handshake"
	"tccelta/backend/internal/domain/protocol"
	"tccelta/backend/internal/domain/timeline"
	"tccelta/backend/internal/port"
	"tccelta/backend/internal/port/porttest"
)

var t0 = time.Unix(1_800_000_000, 0)

const dongleID = 7

func i64(v int64) *int64 { return &v }

type metrics struct {
	porttest.Nop
	boots, alerts map[string]int
	groups        int
	expired       int64
}

func (m *metrics) Boot(s string)   { m.boots[s]++ }
func (m *metrics) Alert(s string)  { m.alerts[s]++ }
func (m *metrics) Groups(n int)    { m.groups += n }
func (m *metrics) Expired(n int64) { m.expired += n }

type env struct {
	db    *memory.Database
	proc  *Processor
	met   *metrics
	clock *porttest.AdjustableClock
}

func newEnv(t *testing.T, neighborTx func(*memory.Database) port.Transactor) *env {
	t.Helper()
	db := memory.New()
	db.AddDongle(dongle.Dongle{ID: dongleID, DevID: protocol.DevID{7}, Profile: protocol.ProfileHMAC, Active: true})
	m := &metrics{boots: map[string]int{}, alerts: map[string]int{}}
	clock := &porttest.AdjustableClock{T: t0}
	var ntx port.Transactor = db
	if neighborTx != nil {
		ntx = neighborTx(db)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &env{db: db, met: m, clock: clock,
		proc: New(db.Repositories(), db, ntx, m, clock.Now, log, Params{Timeline: timeline.DefaultConfig()})}
}

func (e *env) boot(id int32, firstSeq, lastSeq int64, firstT, lastT int64) *memory.Boot {
	b := &memory.Boot{LastSeq: i64(lastSeq)}
	b.BootKey = timeline.BootKey{DongleID: dongleID, Generation: 0, BootID: id}
	b.FirstSeq, b.FirstTMs, b.LastTMs = i64(firstSeq), i64(firstT), i64(lastT)
	e.db.BootRows = append(e.db.BootRows, b)
	return b
}

func (e *env) anchored(b *memory.Boot, a float64, unc int32) {
	src, bb := "anchors", 0.0
	b.AMs, b.B, b.BaseUncertaintyMs, b.TimeSource = &a, &bb, &unc, &src
}

func (e *env) pending(b *memory.Boot, since time.Duration) {
	b.Pending, b.PendingSince = true, t0.Add(since)
}

func TestEmptyQueue(t *testing.T) {
	e := newEnv(t, nil)
	ok, err := e.proc.ProcessOne(context.Background())
	if err != nil || ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

// Boot com âncoras: grava a reta e os grupos, e recoloca na fila o vizinho sem âncora entre dois ancorados.
func TestAnchorsThenNeighbors(t *testing.T) {
	e := newEnv(t, nil)
	ctx := context.Background()
	b1 := e.boot(1, 1, 10, 0, 100_000)
	e.anchored(b1, 1.8e12, 100)
	b2 := e.boot(2, 11, 20, 100_000, 200_000)
	b2.Anchors = []timeline.Anchor{{TMs: 100_000, S1Ms: 1.8e12 + 100_000, S2Ms: 1.8e12 + 100_500},
		{TMs: 200_000, S1Ms: 1.8e12 + 200_000, S2Ms: 1.8e12 + 200_500}}
	e.pending(b2, 0)
	b3 := e.boot(3, 21, 30, 300_000, 400_000) // sem âncora, entre os ancorados 2 e 4
	b4 := e.boot(4, 31, 40, 0, 100_000)
	e.anchored(b4, 1.8e12+1_000_000, 100)

	n, _ := e.proc.PendingCount(ctx)
	if n != 1 {
		t.Fatalf("fila=%d", n)
	}
	ok, err := e.proc.ProcessOne(ctx)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if b2.Pending || b2.TimeSource == nil || *b2.TimeSource != "anchors" || b2.AMs == nil || *b2.BaseUncertaintyMs != 250 {
		t.Fatalf("%+v", b2)
	}
	if g := e.db.Groups[b2.BootKey]; len(g) != 1 || g[0].N != 2 {
		t.Fatalf("grupos %+v", g)
	}
	if e.met.boots["anchors"] != 1 || e.met.groups != 1 {
		t.Fatalf("métricas %+v", e.met)
	}
	if !b3.Pending || b1.Pending || b4.Pending {
		t.Fatalf("vizinhos: b1=%v b3=%v b4=%v", b1.Pending, b3.Pending, b4.Pending)
	}

	// o vizinho agora é calculado pelos boots ancorados ao redor
	ok, err = e.proc.ProcessOne(ctx)
	if err != nil || !ok || b3.Pending || b3.TimeSource == nil || *b3.TimeSource != "neighbors" {
		t.Fatalf("ok=%v err=%v %+v", ok, err, b3)
	}
	if e.met.boots["neighbors"] != 1 {
		t.Fatalf("%+v", e.met.boots)
	}
	if ok, _ := e.proc.ProcessOne(ctx); ok {
		t.Fatal("a fila deveria estar vazia")
	}
}

func TestNoEstimateClearsPending(t *testing.T) {
	e := newEnv(t, nil)
	b := e.boot(1, 1, 10, 0, 1000) // sem âncora e sem vizinhos
	e.pending(b, 0)
	if ok, err := e.proc.ProcessOne(context.Background()); err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if b.Pending || b.TimeSource != nil || b.AMs != nil || e.met.boots["no_estimate"] != 1 {
		t.Fatalf("%+v %+v", b, e.met.boots)
	}
}

// Âncoras incompatíveis geram alerta; recalcular o mesmo boot não repete o alerta.
func TestAlertNotRepeated(t *testing.T) {
	e := newEnv(t, nil)
	b := e.boot(1, 1, 10, 0, 100_000)
	b.Anchors = []timeline.Anchor{{TMs: 0, S1Ms: 1000, S2Ms: 1100}, {TMs: 60_000, S1Ms: 61_500, S2Ms: 61_600}}
	for i := 0; i < 2; i++ {
		e.pending(b, 0)
		if ok, err := e.proc.ProcessOne(context.Background()); err != nil || !ok {
			t.Fatalf("ok=%v err=%v", ok, err)
		}
	}
	if len(e.db.Alerts) != 1 || e.db.Alerts[0].Type != "empty_intersection" || *e.db.Alerts[0].BootID != 1 {
		t.Fatalf("%+v", e.db.Alerts)
	}
	if e.met.alerts["empty_intersection"] != 2 {
		t.Fatalf("a métrica conta cada alerta gerado: %+v", e.met.alerts)
	}
}

// Os boots pendentes saem do mais antigo para o mais novo.
func TestOldestFirst(t *testing.T) {
	e := newEnv(t, nil)
	late, early := e.boot(1, 1, 10, 0, 1000), e.boot(2, 11, 20, 0, 1000)
	e.pending(late, time.Minute)
	e.pending(early, 0)
	if ok, _ := e.proc.ProcessOne(context.Background()); !ok || early.Pending || !late.Pending {
		t.Fatalf("early=%v late=%v", early.Pending, late.Pending)
	}
}

type failingTx struct{}

func (failingTx) InTransaction(context.Context, func(context.Context, port.Repositories) error) error {
	return errors.New("lock_timeout")
}

// Falha ao marcar os vizinhos é só registrada: o boot já foi gravado.
func TestNeighborFailureDoesNotFail(t *testing.T) {
	e := newEnv(t, func(*memory.Database) port.Transactor { return failingTx{} })
	b := e.boot(1, 1, 10, 0, 100_000)
	b.Anchors = []timeline.Anchor{{TMs: 0, S1Ms: 1000, S2Ms: 1100}}
	e.pending(b, 0)
	ok, err := e.proc.ProcessOne(context.Background())
	if err != nil || !ok || b.Pending || b.TimeSource == nil {
		t.Fatalf("ok=%v err=%v %+v", ok, err, b)
	}
}

func TestExpireChallenges(t *testing.T) {
	e := newEnv(t, nil)
	ctx := context.Background()
	_, err := e.db.Repositories().Challenges.Issue(ctx, handshake.Issuance{DongleID: dongleID, S1: t0, MaxPending: 5,
		Nonce: []byte{1}, ExpiresAt: t0.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if n, err := e.proc.ExpireChallenges(ctx); err != nil || n != 0 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	e.clock.Advance(2 * time.Minute)
	if n, err := e.proc.ExpireChallenges(ctx); err != nil || n != 1 || e.met.expired != 1 {
		t.Fatalf("n=%d err=%v expiradas=%d", n, err, e.met.expired)
	}
	if n, _ := e.proc.ExpireChallenges(ctx); n != 0 {
		t.Fatalf("já expirado: %d", n)
	}
}
