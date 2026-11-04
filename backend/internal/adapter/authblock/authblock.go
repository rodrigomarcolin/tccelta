// Package authblock guarda os contadores do bloqueio progressivo de POST /v1/auth/token: no Redis, visto por todas as
// réplicas da API, com um estado local por réplica como reserva quando o Redis está fora (falha aberta).
package authblock

import (
	"context"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"tccelta/backend/internal/domain/clientauth"
	"tccelta/backend/internal/port"
)

// Policies são as progressões do contador do par (client_id, IP) e do contador do IP.
type Policies struct{ Pair, IP clientauth.Policy }

// MaxLocalKeys limita o estado local por réplica.
const MaxLocalKeys = 10000

const (
	pairPrefix = "tccelta:tok:p:"
	ipPrefix   = "tccelta:tok:i:"
)

// Redis é o que o armazenamento precisa do cliente Redis (*redis.Client satisfaz).
type Redis interface {
	redis.Scripter
	Del(ctx context.Context, keys ...string) *redis.IntCmd
}

// Store implementa port.AuthBlocker: tenta o Redis e, se ele falhar, usa o estado local.
type Store struct {
	pol     Policies
	redis   Redis // nil: só o estado local (dev simples)
	timeout time.Duration
	local   *Local
	met     port.MetricsAuth
	check   *redis.Script
	fail    *redis.Script
}

// New cria o armazenamento. rc pode ser nil (sem Redis, só o estado local); met pode ser nil.
func New(rc Redis, timeout time.Duration, pol Policies, met port.MetricsAuth) *Store {
	return &Store{pol: pol, redis: rc, timeout: timeout, local: NewLocal(pol, time.Now), met: met,
		check: redis.NewScript(luaCheck), fail: redis.NewScript(luaFail)}
}

// Local devolve o estado local de reserva.
func (s *Store) Local() *Local { return s.local }

// Atomicidade: cada script é uma só ida ao Redis e usa o TIME do próprio Redis, então as réplicas não dependem do
// relógio uma da outra. O estado de cada chave é um hash {n: falhas, u: fim do bloqueio em ms}.
//
// luaCheck devolve, para cada chave, o tempo restante de bloqueio em ms.
const luaCheck = `local t = redis.call('TIME')
local now = t[1] * 1000 + math.floor(t[2] / 1000)
local out = {}
for i, k in ipairs(KEYS) do
  local u = tonumber(redis.call('HGET', k, 'u')) or 0
  local rem = u - now
  if rem < 0 then rem = 0 end
  out[i] = rem
end
return out`

// luaFail incrementa o contador de cada chave, calcula o novo bloqueio (mesma fórmula de clientauth.Policy.Block) e renova o
// esquecimento para (bloqueio restante + ESQUECER). ARGV: inicial_ms, fator, teto_ms, esquecer_ms, tolerância de cada chave.
// Devolve {n, bloqueio_novo_ms} por chave.
const luaFail = `local t = redis.call('TIME')
local now = t[1] * 1000 + math.floor(t[2] / 1000)
local initial, factor, cap, forget = tonumber(ARGV[1]), tonumber(ARGV[2]), tonumber(ARGV[3]), tonumber(ARGV[4])
local out = {}
for i, k in ipairs(KEYS) do
  local tol = tonumber(ARGV[4 + i])
  local n = redis.call('HINCRBY', k, 'n', 1)
  local block = 0
  if n > tol then
    local d = initial
    local e = n - tol - 1
    if factor > 1 then
      while e > 0 and d < cap do d = d * factor; e = e - 1 end
    end
    if d > cap then d = cap end
    block = math.floor(d)
  end
  local u = tonumber(redis.call('HGET', k, 'u')) or 0
  local rem = u - now
  if rem < 0 then rem = 0 end
  if block > 0 then
    redis.call('HSET', k, 'u', now + block)
    if block > rem then rem = block end
  end
  redis.call('PEXPIRE', k, rem + forget)
  out[2 * i - 1] = n
  out[2 * i] = block
end
return out`

func (s *Store) ctx(ctx context.Context) (context.Context, context.CancelFunc) {
	if s.timeout > 0 {
		return context.WithTimeout(ctx, s.timeout)
	}
	return ctx, func() {}
}

func (s *Store) storeFailed() {
	if s.met != nil {
		s.met.BlockStoreFailure()
	}
}

func keys(pair, ip string) []string {
	if pair == "" {
		return []string{ipPrefix + ip}
	}
	return []string{pairPrefix + pair, ipPrefix + ip}
}

// Check implementa port.AuthBlocker.
func (s *Store) Check(ctx context.Context, pair, ip string) time.Duration {
	if s.redis != nil {
		c, cancel := s.ctx(ctx)
		defer cancel()
		res, err := s.check.Run(c, s.redis, keys(pair, ip)).Int64Slice()
		if err == nil {
			var longest int64
			for _, r := range res {
				if r > longest {
					longest = r
				}
			}
			return time.Duration(longest) * time.Millisecond
		}
		s.storeFailed()
	}
	return s.local.Check(pair, ip)
}

// Fail implementa port.AuthBlocker.
func (s *Store) Fail(ctx context.Context, pair, ip string) port.AuthBlockOutcome {
	if s.redis != nil {
		c, cancel := s.ctx(ctx)
		defer cancel()
		ks := keys(pair, ip)
		p := s.pol.IP
		argv := []any{p.Initial.Milliseconds(), p.Factor, p.Cap.Milliseconds(), p.Forget.Milliseconds()}
		if pair != "" {
			argv = append(argv, s.pol.Pair.Tolerance)
		}
		argv = append(argv, s.pol.IP.Tolerance)
		res, err := s.fail.Run(c, s.redis, ks, argv...).Int64Slice()
		if err == nil && len(res) == 2*len(ks) {
			var o port.AuthBlockOutcome
			if pair != "" {
				o.PairCount, o.PairBlock = int(res[0]), time.Duration(res[1])*time.Millisecond
				res = res[2:]
			}
			o.IPCount, o.IPBlock = int(res[0]), time.Duration(res[1])*time.Millisecond
			return o
		}
		s.storeFailed()
	}
	return s.local.Fail(pair, ip)
}

// Reset implementa port.AuthBlocker: apaga só o contador do par.
func (s *Store) Reset(ctx context.Context, pair string) {
	if pair == "" {
		return
	}
	if s.redis != nil {
		c, cancel := s.ctx(ctx)
		defer cancel()
		if err := s.redis.Del(c, pairPrefix+pair).Err(); err == nil {
			return
		}
		s.storeFailed()
	}
	s.local.Reset(pair)
}

var _ port.AuthBlocker = (*Store)(nil)

// Local é o estado em memória de uma réplica: o mesmo algoritmo do Redis, limitado a MaxLocalKeys chaves.
type Local struct {
	pol Policies
	now func() time.Time
	mu  sync.Mutex
	m   map[string]*entry
}

type entry struct {
	n             int
	until, expire time.Time
}

// NewLocal cria o estado local.
func NewLocal(pol Policies, now func() time.Time) *Local {
	return &Local{pol: pol, now: now, m: map[string]*entry{}}
}

func (l *Local) live(k string, now time.Time) *entry {
	if e := l.m[k]; e != nil && now.Before(e.expire) {
		return e
	}
	delete(l.m, k)
	return nil
}

// Check devolve o maior bloqueio restante entre o par e o IP.
func (l *Local) Check(pair, ip string) time.Duration {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	var longest time.Duration
	for _, k := range keys(pair, ip) {
		if e := l.live(k, now); e != nil {
			if r := e.until.Sub(now); r > longest {
				longest = r
			}
		}
	}
	return longest
}

// Fail registra a falha no par (se não vazio) e no IP.
func (l *Local) Fail(pair, ip string) port.AuthBlockOutcome {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	var o port.AuthBlockOutcome
	if pair != "" {
		o.PairCount, o.PairBlock = l.bump(pairPrefix+pair, l.pol.Pair, now)
	}
	o.IPCount, o.IPBlock = l.bump(ipPrefix+ip, l.pol.IP, now)
	return o
}

func (l *Local) bump(k string, pol clientauth.Policy, now time.Time) (int, time.Duration) {
	e := l.live(k, now)
	if e == nil {
		if len(l.m) >= MaxLocalKeys {
			l.evict(now)
		}
		if len(l.m) >= MaxLocalKeys {
			return 0, 0 // cheio de bloqueios ativos: não rastreia a chave nova (falha aberta)
		}
		e = &entry{}
		l.m[k] = e
	}
	e.n++
	block := pol.Block(e.n)
	rem := e.until.Sub(now)
	if rem < 0 {
		rem = 0
	}
	if block > 0 {
		e.until = now.Add(block)
		if block > rem {
			rem = block
		}
	}
	e.expire = now.Add(rem + pol.Forget)
	return e.n, block
}

// evict tira o que já foi esquecido e, se ainda estiver cheio, o que não está bloqueado.
func (l *Local) evict(now time.Time) {
	for k, e := range l.m {
		if !now.Before(e.expire) {
			delete(l.m, k)
		}
	}
	if len(l.m) < MaxLocalKeys {
		return
	}
	for k, e := range l.m {
		if !now.Before(e.until) {
			delete(l.m, k)
		}
	}
}

// Len diz quantas chaves o estado local guarda.
func (l *Local) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.m)
}

// Reset apaga o contador do par.
func (l *Local) Reset(pair string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.m, pairPrefix+pair)
}
