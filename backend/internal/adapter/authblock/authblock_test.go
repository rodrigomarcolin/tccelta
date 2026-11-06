package authblock

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"tccelta/backend/internal/domain/clientauth"
	"tccelta/backend/internal/port"
)

var (
	pairPolicy = clientauth.Policy{Tolerance: 5, Initial: 30 * time.Second, Factor: 2, Cap: 15 * time.Minute, Forget: 30 * time.Minute}
	ipPolicy   = clientauth.Policy{Tolerance: 20, Initial: 30 * time.Second, Factor: 2, Cap: 15 * time.Minute, Forget: 30 * time.Minute}
	policies   = Policies{Pair: pairPolicy, IP: ipPolicy}
)

type metrics struct{ storeFailures int }

func (m *metrics) TokenRequest(string) {}
func (m *metrics) Block(string)        {}
func (m *metrics) BlockStoreFailure()  { m.storeFailures++ }

var epoch = time.Unix(1_800_000_000, 0)

// newRedisStore liga um Store a um miniredis com o relógio em epoch. O teste avança o tempo com advance.
func newRedisStore(t *testing.T, pol Policies, met port.MetricsAuth) (*Store, *clocked) {
	t.Helper()
	mr := miniredis.RunT(t)
	mr.SetTime(epoch)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = rc.Close() })
	return New(rc, time.Second, pol, met), &clocked{Miniredis: mr, cur: epoch}
}

// clocked é o miniredis com o relógio (usado pelo TIME do Lua) que o teste controla.
type clocked struct {
	*miniredis.Miniredis
	cur time.Time
}

func advance(mr *clocked, d time.Duration) {
	mr.cur = mr.cur.Add(d)
	mr.SetTime(mr.cur)
	mr.FastForward(d)
}

func TestRedisProgressionPerPair(t *testing.T) {
	s, mr := newRedisStore(t, policies, nil)
	ctx := context.Background()
	const pair, ip = "next-dev|10.0.0.1", "10.0.0.1"
	if s.Check(ctx, pair, ip) != 0 {
		t.Fatal("livre no começo")
	}
	for n := 1; n <= 5; n++ {
		o := s.Fail(ctx, pair, ip)
		if o.PairCount != n || o.PairBlock != 0 || o.IPCount != n || o.IPBlock != 0 {
			t.Fatalf("falha %d: %+v", n, o)
		}
		if s.Check(ctx, pair, ip) != 0 {
			t.Fatalf("falha %d não bloqueia", n)
		}
	}
	// 6ª: 30 s, 7ª: 60 s, ... , teto de 15 min
	for i, want := range []time.Duration{30 * time.Second, 60 * time.Second, 120 * time.Second, 240 * time.Second, 480 * time.Second,
		15 * time.Minute, 15 * time.Minute} {
		o := s.Fail(ctx, pair, ip)
		if o.PairCount != 6+i || o.PairBlock != want {
			t.Fatalf("falha %d: %+v, bloqueio esperado %v", 6+i, o, want)
		}
		if got := s.Check(ctx, pair, ip); got != want {
			t.Fatalf("Check logo após a falha %d: %v, esperava %v", 6+i, got, want)
		}
		advance(mr, want) // espera o bloqueio acabar (o contador segue: o esquecimento é de 30 min)
		if s.Check(ctx, pair, ip) != 0 {
			t.Fatalf("depois de %v o bloqueio acaba", want)
		}
	}
}

func TestRedisBlockIsNotExtendedByChecks(t *testing.T) {
	s, mr := newRedisStore(t, policies, nil)
	ctx := context.Background()
	for i := 0; i < 6; i++ {
		s.Fail(ctx, "a-b-c|1.1.1.1", "1.1.1.1")
	}
	a := s.Check(ctx, "a-b-c|1.1.1.1", "1.1.1.1")
	advance(mr, 10*time.Second)
	for i := 0; i < 5; i++ { // tentativas bloqueadas: só Check, nunca Fail
		s.Check(ctx, "a-b-c|1.1.1.1", "1.1.1.1")
	}
	b := s.Check(ctx, "a-b-c|1.1.1.1", "1.1.1.1")
	if a != 30*time.Second || b != 20*time.Second {
		t.Fatalf("%v %v", a, b)
	}
}

func TestRedisIPCounterAcrossClientIDs(t *testing.T) {
	s, _ := newRedisStore(t, policies, nil)
	ctx := context.Background()
	const ip = "10.0.0.9"
	for n := 1; n <= 20; n++ {
		// cada falha com um client_id diferente: nenhum par passa da tolerância
		o := s.Fail(ctx, fmt.Sprintf("cli-%02d|%s", n, ip), ip)
		if o.PairBlock != 0 || o.IPBlock != 0 || o.IPCount != n {
			t.Fatalf("falha %d: %+v", n, o)
		}
	}
	if o := s.Fail(ctx, "cli-21|"+ip, ip); o.IPBlock != 30*time.Second || o.PairBlock != 0 {
		t.Fatalf("a 21ª falha do IP bloqueia: %+v", o)
	}
	// o par que nunca falhou também fica bloqueado pelo IP, mas outro IP segue livre
	if got := s.Check(ctx, "novo-id|"+ip, ip); got != 30*time.Second {
		t.Fatalf("%v", got)
	}
	if got := s.Check(ctx, "novo-id|10.0.0.10", "10.0.0.10"); got != 0 {
		t.Fatalf("outro IP: %v", got)
	}
	// sem par (Basic ausente ou client_id malformado): só o IP
	if o := s.Fail(ctx, "", "10.0.0.11"); o.IPCount != 1 || o.PairCount != 0 {
		t.Fatalf("%+v", o)
	}
	if got := s.Check(ctx, "", ip); got != 30*time.Second {
		t.Fatalf("Check só do IP: %v", got)
	}
}

func TestRedisPairsAreIndependentPerIP(t *testing.T) {
	s, _ := newRedisStore(t, policies, nil)
	ctx := context.Background()
	for i := 0; i < 6; i++ {
		s.Fail(ctx, "next-dev|1.1.1.1", "1.1.1.1")
	}
	if s.Check(ctx, "next-dev|1.1.1.1", "1.1.1.1") == 0 {
		t.Fatal("(a, IP1) bloqueado")
	}
	if got := s.Check(ctx, "next-dev|2.2.2.2", "2.2.2.2"); got != 0 {
		t.Fatalf("(a, IP2) não pode ser afetado: %v", got)
	}
}

func TestRedisResetClearsOnlyPair(t *testing.T) {
	s, _ := newRedisStore(t, policies, nil)
	ctx := context.Background()
	const pair, ip = "next-dev|1.1.1.1", "1.1.1.1"
	for i := 0; i < 3; i++ {
		s.Fail(ctx, pair, ip)
	}
	s.Reset(ctx, pair)
	o := s.Fail(ctx, pair, ip)
	if o.PairCount != 1 || o.IPCount != 4 {
		t.Fatalf("o par recomeça e o IP continua: %+v", o)
	}
	s.Reset(ctx, "") // sem par: nada a fazer
}

func TestRedisForgetsAfterQuietPeriod(t *testing.T) {
	s, mr := newRedisStore(t, policies, nil)
	ctx := context.Background()
	const pair, ip = "next-dev|1.1.1.1", "1.1.1.1"
	for i := 0; i < 6; i++ {
		s.Fail(ctx, pair, ip)
	}
	// bloqueio de 30 s + 30 min de esquecimento: antes disso o contador existe, depois some
	advance(mr, 30*time.Minute)
	if o := s.Fail(ctx, pair, ip); o.PairCount != 7 || o.PairBlock != 60*time.Second {
		t.Fatalf("ainda lembra: %+v", o)
	}
	advance(mr, 61*time.Second+30*time.Minute)
	if o := s.Fail(ctx, pair, ip); o.PairCount != 1 || o.PairBlock != 0 {
		t.Fatalf("esqueceu: %+v", o)
	}
}

// As duas réplicas da API compartilham o estado: falhas somadas nas duas bloqueiam ambas.
func TestTwoReplicasShareState(t *testing.T) {
	mr := miniredis.RunT(t)
	mr.SetTime(epoch)
	c1 := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	c2 := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	a, b := New(c1, time.Second, policies, nil), New(c2, time.Second, policies, nil)
	ctx := context.Background()
	const pair, ip = "next-dev|1.1.1.1", "1.1.1.1"
	for i := 0; i < 3; i++ {
		a.Fail(ctx, pair, ip)
		b.Fail(ctx, pair, ip)
	}
	if a.Check(ctx, pair, ip) == 0 || b.Check(ctx, pair, ip) == 0 {
		t.Fatal("6 falhas somadas nas duas réplicas bloqueiam as duas")
	}
}

// Redis fora: o algoritmo roda no estado local da réplica e a métrica de falha do armazenamento cresce.
func TestRedisDownFallsBackToLocal(t *testing.T) {
	met := &metrics{}
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1, DialTimeout: 100 * time.Millisecond})
	s := New(rc, 200*time.Millisecond, policies, met)
	now := epoch
	s.local = NewLocal(policies, func() time.Time { return now })
	mr.Close()
	ctx := context.Background()
	const pair, ip = "next-dev|1.1.1.1", "1.1.1.1"
	if s.Check(ctx, pair, ip) != 0 || met.storeFailures == 0 {
		t.Fatalf("Check com Redis fora: falhas=%d", met.storeFailures)
	}
	before := met.storeFailures
	for i := 1; i <= 5; i++ {
		if o := s.Fail(ctx, pair, ip); o.PairCount != i || o.PairBlock != 0 {
			t.Fatalf("falha %d: %+v", i, o)
		}
	}
	if o := s.Fail(ctx, pair, ip); o.PairBlock != 30*time.Second {
		t.Fatalf("o bloqueio vale na réplica: %+v", o)
	}
	if got := s.Check(ctx, pair, ip); got != 30*time.Second {
		t.Fatalf("%v", got)
	}
	s.Reset(ctx, pair)
	if got := s.Check(ctx, pair, ip); got != 0 {
		t.Fatalf("o par zerou, mas o IP segue contado: %v", got)
	}
	if met.storeFailures <= before {
		t.Fatalf("tccelta_auth_block_store_failures_total deveria crescer: %d", met.storeFailures)
	}
}

func TestWithoutRedisOnlyLocal(t *testing.T) {
	s := New(nil, 0, policies, nil)
	ctx := context.Background()
	for i := 0; i < 6; i++ {
		s.Fail(ctx, "next-dev|1.1.1.1", "1.1.1.1")
	}
	if s.Check(ctx, "next-dev|1.1.1.1", "1.1.1.1") == 0 {
		t.Fatal("o estado local bloqueia")
	}
	s.Reset(ctx, "next-dev|1.1.1.1")
	if s.Check(ctx, "next-dev|1.1.1.1", "1.1.1.1") != 0 {
		t.Fatal("Reset local")
	}
	if s.Local().Len() == 0 {
		t.Fatal("o contador do IP continua")
	}
}

// A fórmula do Lua e a de clientauth.Policy.Block (usada pelo estado local) dão o mesmo resultado.
func TestLuaMatchesPolicyBlock(t *testing.T) {
	for _, factor := range []float64{1, 1.5, 2, 3} {
		pol := Policies{
			Pair: clientauth.Policy{Tolerance: 2, Initial: 10 * time.Second, Factor: factor, Cap: 5 * time.Minute, Forget: time.Hour},
			IP:   clientauth.Policy{Tolerance: 3, Initial: 10 * time.Second, Factor: factor, Cap: 5 * time.Minute, Forget: time.Hour},
		}
		s, mr := newRedisStore(t, pol, nil)
		ctx := context.Background()
		for n := 1; n <= 14; n++ {
			o := s.Fail(ctx, "next-dev|1.1.1.1", "1.1.1.1")
			if want := pol.Pair.Block(n); diff(o.PairBlock, want) > time.Millisecond {
				t.Fatalf("fator %v, falha %d: Lua %v, Go %v", factor, n, o.PairBlock, want)
			}
			if want := pol.IP.Block(n); diff(o.IPBlock, want) > time.Millisecond {
				t.Fatalf("fator %v, IP, falha %d: Lua %v, Go %v", factor, n, o.IPBlock, want)
			}
			advance(mr, 5*time.Minute)
		}
	}
}

func diff(a, b time.Duration) time.Duration {
	if a < b {
		return b - a
	}
	return a - b
}

func TestLocalCapsKeysAndKeepsActiveBlocks(t *testing.T) {
	pol := Policies{
		Pair: clientauth.Policy{Tolerance: 1, Initial: time.Minute, Factor: 2, Cap: time.Hour, Forget: time.Hour},
		IP:   clientauth.Policy{Tolerance: 1, Initial: time.Minute, Factor: 2, Cap: time.Hour, Forget: time.Hour},
	}
	now := epoch
	l := NewLocal(pol, func() time.Time { return now })
	// enche com chaves sem bloqueio (1 falha cada): ao passar do limite, as ociosas saem
	for i := 0; i < MaxLocalKeys+50; i++ {
		l.Fail("", fmt.Sprintf("ip-%d", i))
	}
	if l.Len() > MaxLocalKeys {
		t.Fatalf("limite de chaves: %d", l.Len())
	}
	// agora todas bloqueadas: nenhuma é expulsa e a chave nova não é rastreada (falha aberta)
	l2 := NewLocal(pol, func() time.Time { return now })
	for i := 0; i < MaxLocalKeys; i++ {
		ip := fmt.Sprintf("ip-%d", i)
		l2.Fail("", ip)
		l2.Fail("", ip) // 2ª falha passa da tolerância 1
	}
	if l2.Len() != MaxLocalKeys {
		t.Fatalf("%d", l2.Len())
	}
	if o := l2.Fail("", "ip-novo"); o.IPCount != 0 || l2.Len() != MaxLocalKeys {
		t.Fatalf("chave nova com o armazenamento cheio de bloqueios: %+v %d", o, l2.Len())
	}
	if l2.Check("", "ip-0") == 0 {
		t.Fatal("o bloqueio ativo foi preservado")
	}
	// o tempo passa: tudo é esquecido e a chave nova entra
	now = now.Add(3 * time.Hour)
	if o := l2.Fail("", "ip-novo"); o.IPCount != 1 {
		t.Fatalf("%+v", o)
	}
}

func TestLocalExpiryAndRemainingBlock(t *testing.T) {
	now := epoch
	l := NewLocal(policies, func() time.Time { return now })
	for i := 0; i < 6; i++ {
		l.Fail("a-b-c|1.1.1.1", "1.1.1.1")
	}
	if got := l.Check("a-b-c|1.1.1.1", "1.1.1.1"); got != 30*time.Second {
		t.Fatalf("%v", got)
	}
	now = now.Add(31 * time.Second)
	if l.Check("a-b-c|1.1.1.1", "1.1.1.1") != 0 {
		t.Fatal("bloqueio acabou")
	}
	if o := l.Fail("a-b-c|1.1.1.1", "1.1.1.1"); o.PairCount != 7 || o.PairBlock != 60*time.Second {
		t.Fatalf("o contador segue até o esquecimento: %+v", o)
	}
	now = now.Add(2 * time.Hour)
	if o := l.Fail("a-b-c|1.1.1.1", "1.1.1.1"); o.PairCount != 1 {
		t.Fatalf("esquecido: %+v", o)
	}
}
