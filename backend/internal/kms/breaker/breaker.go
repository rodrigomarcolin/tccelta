// Package breaker é o disjuntor (circuit breaker) das chamadas ao Vault.
package breaker

import (
	"sync"
	"time"
)

// Breaker: `limite` falhas seguidas abrem por `espera`; depois, meia-abertura com 1 sonda.
type Breaker struct {
	mu        sync.Mutex
	failures  int
	limit     int
	wait      time.Duration
	openUntil time.Time
	probe     bool
	now       func() time.Time
}

// New cria o disjuntor (o padrão do serviço é Novo(5, 10*time.Second)).
func New(limit int, wait time.Duration) *Breaker {
	return &Breaker{limit: limit, wait: wait, now: time.Now}
}

// Allow diz se a chamada pode seguir.
func (d *Breaker) Allow() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.failures < d.limit {
		return true
	}
	if d.now().Before(d.openUntil) {
		return false
	}
	if d.probe { // meia-abertura: só uma sonda por vez
		return false
	}
	d.probe = true
	return true
}

// Success fecha o disjuntor.
func (d *Breaker) Success() {
	d.mu.Lock()
	d.failures, d.probe = 0, false
	d.mu.Unlock()
}

// Failure conta uma falha e abre o disjuntor ao atingir o limite.
func (d *Breaker) Failure() {
	d.mu.Lock()
	d.failures++
	d.probe = false
	if d.failures >= d.limit {
		d.openUntil = d.now().Add(d.wait)
	}
	d.mu.Unlock()
}

// Open diz se o disjuntor está recusando chamadas.
func (d *Breaker) Open() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.failures >= d.limit && d.now().Before(d.openUntil)
}
