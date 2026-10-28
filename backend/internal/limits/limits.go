// Package limites implementa o rate limit (local + Redis global) e o semáforo de concorrência.
package limites

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Limitador decide se uma chave pode prosseguir dentro do limite por minuto.
type Limitador interface {
	Permitir(ctx context.Context, escopo, chave string, limiteMin int) (ok bool, retryAfter time.Duration)
}

func restoDoMinuto(t time.Time) time.Duration {
	r := time.Duration(60-t.Second())*time.Second - time.Duration(t.Nanosecond())
	if r < time.Second {
		r = time.Second
	}
	return r
}

// LimitadorLocal: contador de janela fixa de 1 min por réplica.
type LimitadorLocal struct {
	Agora func() time.Time
	mu    sync.Mutex
	m     map[string]*janela
}

type janela struct {
	minuto int64
	n      int
}

// NovoLocal cria o limitador local.
func NovoLocal() *LimitadorLocal { return &LimitadorLocal{Agora: time.Now, m: map[string]*janela{}} }

// Permitir implementa Limitador.
func (l *LimitadorLocal) Permitir(_ context.Context, escopo, chave string, limite int) (bool, time.Duration) {
	now := l.Agora()
	min := now.Unix() / 60
	k := escopo + ":" + chave
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.m) > 100000 {
		l.m = map[string]*janela{}
	}
	j := l.m[k]
	if j == nil || j.minuto != min {
		j = &janela{minuto: min}
		l.m[k] = j
	}
	j.n++
	if j.n > limite {
		return false, restoDoMinuto(now)
	}
	return true, 0
}

const luaIncr = `local n = redis.call('INCR', KEYS[1])
if n == 1 then redis.call('PEXPIRE', KEYS[1], ARGV[1]) end
return n`

// LimitadorRedis: contador global de janela fixa de 1 min.
type LimitadorRedis struct {
	C       redis.Scripter
	Agora   func() time.Time
	Timeout time.Duration
	script  *redis.Script
}

// NovoRedis cria o limitador Redis.
func NovoRedis(c redis.Scripter, timeout time.Duration) *LimitadorRedis {
	return &LimitadorRedis{C: c, Agora: time.Now, Timeout: timeout, script: redis.NewScript(luaIncr)}
}

// PermitirErr devolve erro se o Redis falhar.
func (l *LimitadorRedis) PermitirErr(ctx context.Context, escopo, chave string, limite int) (bool, time.Duration, error) {
	now := l.Agora()
	if l.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, l.Timeout)
		defer cancel()
	}
	k := fmt.Sprintf("tccelta:rl:%s:%s:%d", escopo, chave, now.Unix()/60)
	n, err := l.script.Run(ctx, l.C, []string{k}, 70000).Int()
	if err != nil {
		return true, 0, err
	}
	if n > limite {
		return false, restoDoMinuto(now), nil
	}
	return true, 0, nil
}

// Permitir implementa Limitador (sem fallback: erro vira permitido).
func (l *LimitadorRedis) Permitir(ctx context.Context, escopo, chave string, limite int) (bool, time.Duration) {
	ok, ra, _ := l.PermitirErr(ctx, escopo, chave, limite)
	return ok, ra
}

// LimitadorComFallback aplica o limite local e depois o global; falha do Redis nunca bloqueia.
type LimitadorComFallback struct {
	Local    *LimitadorLocal
	Redis    *LimitadorRedis // pode ser nil
	OnFalha  func()
	Agora    func() time.Time
	mu       sync.Mutex
	falhas   int
	abertoAt time.Time
}

// Permitir implementa Limitador.
func (l *LimitadorComFallback) Permitir(ctx context.Context, escopo, chave string, limite int) (bool, time.Duration) {
	if ok, ra := l.Local.Permitir(ctx, escopo, chave, limite); !ok {
		return false, ra
	}
	if l.Redis == nil {
		return true, 0
	}
	agora := time.Now
	if l.Agora != nil {
		agora = l.Agora
	}
	l.mu.Lock()
	if l.falhas >= 5 && agora().Before(l.abertoAt) {
		l.mu.Unlock()
		return true, 0
	}
	l.mu.Unlock()
	ok, ra, err := l.Redis.PermitirErr(ctx, escopo, chave, limite)
	l.mu.Lock()
	defer l.mu.Unlock()
	if err != nil {
		l.falhas++
		if l.falhas >= 5 {
			l.abertoAt = agora().Add(10 * time.Second)
		}
		if l.OnFalha != nil {
			l.OnFalha()
		}
		return true, 0
	}
	l.falhas = 0
	return ok, ra
}

// Concorrencia é o semáforo da ingestão (TryAcquire com espera curta).
type Concorrencia struct {
	vagas  chan struct{}
	espera time.Duration
}

// NovaConcorrencia cria o semáforo com n vagas.
func NovaConcorrencia(n int, espera time.Duration) *Concorrencia {
	return &Concorrencia{vagas: make(chan struct{}, n), espera: espera}
}

// Adquirir tenta pegar uma vaga; devolve a função de liberação ou false.
func (c *Concorrencia) Adquirir() (func(), bool) {
	select {
	case c.vagas <- struct{}{}:
		return func() { <-c.vagas }, true
	default:
	}
	if c.espera <= 0 {
		return nil, false
	}
	t := time.NewTimer(c.espera)
	defer t.Stop()
	select {
	case c.vagas <- struct{}{}:
		return func() { <-c.vagas }, true
	case <-t.C:
		return nil, false
	}
}
