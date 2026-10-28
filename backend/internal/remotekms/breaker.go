package remotekms

import (
	"sync"
	"time"
)

// breaker: 5 falhas seguidas abrem por 10 s; depois, meia-abertura com 1 sonda.
// Cópia intencional do disjuntor do serviço KMS: os dois módulos não compartilham código.
type breaker struct {
	mu        sync.Mutex
	failures  int
	limit     int
	wait      time.Duration
	openUntil time.Time
	probe     bool
	now       func() time.Time
}

func newBreaker(limit int, wait time.Duration) *breaker {
	return &breaker{limit: limit, wait: wait, now: time.Now}
}

// allow diz se a chamada pode seguir.
func (d *breaker) allow() bool {
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

func (d *breaker) success() {
	d.mu.Lock()
	d.failures, d.probe = 0, false
	d.mu.Unlock()
}

func (d *breaker) failure() {
	d.mu.Lock()
	d.failures++
	d.probe = false
	if d.failures >= d.limit {
		d.openUntil = d.now().Add(d.wait)
	}
	d.mu.Unlock()
}

func (d *breaker) open() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.failures >= d.limit && d.now().Before(d.openUntil)
}
