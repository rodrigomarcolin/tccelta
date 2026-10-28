// Package limits implementa o rate limit (local + Redis global) e o semáforo de concorrência.
package limits

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Limiter decide se uma chave pode prosseguir dentro do limite por minuto.
type Limiter interface {
	Allow(ctx context.Context, scope, key string, limitMin int) (ok bool, retryAfter time.Duration)
}

func minuteRemainder(t time.Time) time.Duration {
	r := time.Duration(60-t.Second())*time.Second - time.Duration(t.Nanosecond())
	if r < time.Second {
		r = time.Second
	}
	return r
}

// LocalLimiter: contador de janela fixa de 1 min por réplica.
type LocalLimiter struct {
	Now func() time.Time
	mu  sync.Mutex
	m   map[string]*window
}

type window struct {
	minute int64
	n      int
}

// NewLocal cria o limitador local.
func NewLocal() *LocalLimiter { return &LocalLimiter{Now: time.Now, m: map[string]*window{}} }

// Allow implementa Limiter.
func (l *LocalLimiter) Allow(_ context.Context, scope, key string, limit int) (bool, time.Duration) {
	now := l.Now()
	min := now.Unix() / 60
	k := scope + ":" + key
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.m) > 100000 {
		l.m = map[string]*window{}
	}
	j := l.m[k]
	if j == nil || j.minute != min {
		j = &window{minute: min}
		l.m[k] = j
	}
	j.n++
	if j.n > limit {
		return false, minuteRemainder(now)
	}
	return true, 0
}

const luaIncr = `local n = redis.call('INCR', KEYS[1])
if n == 1 then redis.call('PEXPIRE', KEYS[1], ARGV[1]) end
return n`

// RedisLimiter: contador global de janela fixa de 1 min.
type RedisLimiter struct {
	C       redis.Scripter
	Now     func() time.Time
	Timeout time.Duration
	script  *redis.Script
}

// NewRedis cria o limitador Redis.
func NewRedis(c redis.Scripter, timeout time.Duration) *RedisLimiter {
	return &RedisLimiter{C: c, Now: time.Now, Timeout: timeout, script: redis.NewScript(luaIncr)}
}

// AllowErr devolve erro se o Redis falhar.
func (l *RedisLimiter) AllowErr(ctx context.Context, scope, key string, limit int) (bool, time.Duration, error) {
	now := l.Now()
	if l.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, l.Timeout)
		defer cancel()
	}
	k := fmt.Sprintf("tccelta:rl:%s:%s:%d", scope, key, now.Unix()/60)
	n, err := l.script.Run(ctx, l.C, []string{k}, 70000).Int()
	if err != nil {
		return true, 0, err
	}
	if n > limit {
		return false, minuteRemainder(now), nil
	}
	return true, 0, nil
}

// Allow implementa Limiter (sem fallback: erro vira permitido).
func (l *RedisLimiter) Allow(ctx context.Context, scope, key string, limit int) (bool, time.Duration) {
	ok, ra, _ := l.AllowErr(ctx, scope, key, limit)
	return ok, ra
}

// LimiterWithFallback aplica o limite local e depois o global; falha do Redis nunca bloqueia.
type LimiterWithFallback struct {
	Local     *LocalLimiter
	Redis     *RedisLimiter // pode ser nil
	OnFailure func()
	Now       func() time.Time
	mu        sync.Mutex
	failures  int
	openedAt  time.Time
}

// Allow implementa Limiter.
func (l *LimiterWithFallback) Allow(ctx context.Context, scope, key string, limit int) (bool, time.Duration) {
	if ok, ra := l.Local.Allow(ctx, scope, key, limit); !ok {
		return false, ra
	}
	if l.Redis == nil {
		return true, 0
	}
	now := time.Now
	if l.Now != nil {
		now = l.Now
	}
	l.mu.Lock()
	if l.failures >= 5 && now().Before(l.openedAt) {
		l.mu.Unlock()
		return true, 0
	}
	l.mu.Unlock()
	ok, ra, err := l.Redis.AllowErr(ctx, scope, key, limit)
	l.mu.Lock()
	defer l.mu.Unlock()
	if err != nil {
		l.failures++
		if l.failures >= 5 {
			l.openedAt = now().Add(10 * time.Second)
		}
		if l.OnFailure != nil {
			l.OnFailure()
		}
		return true, 0
	}
	l.failures = 0
	return ok, ra
}

// Concurrency é o semáforo da ingestão (TryAcquire com espera curta).
type Concurrency struct {
	slots chan struct{}
	wait  time.Duration
}

// NewConcurrency cria o semáforo com n vagas.
func NewConcurrency(n int, wait time.Duration) *Concurrency {
	return &Concurrency{slots: make(chan struct{}, n), wait: wait}
}

// Acquire tenta pegar uma vaga; devolve a função de liberação ou false.
func (c *Concurrency) Acquire() (func(), bool) {
	select {
	case c.slots <- struct{}{}:
		return func() { <-c.slots }, true
	default:
	}
	if c.wait <= 0 {
		return nil, false
	}
	t := time.NewTimer(c.wait)
	defer t.Stop()
	select {
	case c.slots <- struct{}{}:
		return func() { <-c.slots }, true
	case <-t.C:
		return nil, false
	}
}
