package kmsremoto

import (
	"sync"
	"time"
)

// disjuntor: 5 falhas seguidas abrem por 10 s; depois, meia-abertura com 1 sonda.
// Cópia intencional do disjuntor do serviço KMS: os dois módulos não compartilham código.
type disjuntor struct {
	mu        sync.Mutex
	falhas    int
	limite    int
	espera    time.Duration
	abertoAte time.Time
	sonda     bool
	agora     func() time.Time
}

func novoDisjuntor(limite int, espera time.Duration) *disjuntor {
	return &disjuntor{limite: limite, espera: espera, agora: time.Now}
}

// permitir diz se a chamada pode seguir.
func (d *disjuntor) permitir() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.falhas < d.limite {
		return true
	}
	if d.agora().Before(d.abertoAte) {
		return false
	}
	if d.sonda { // meia-abertura: só uma sonda por vez
		return false
	}
	d.sonda = true
	return true
}

func (d *disjuntor) sucesso() {
	d.mu.Lock()
	d.falhas, d.sonda = 0, false
	d.mu.Unlock()
}

func (d *disjuntor) falha() {
	d.mu.Lock()
	d.falhas++
	d.sonda = false
	if d.falhas >= d.limite {
		d.abertoAte = d.agora().Add(d.espera)
	}
	d.mu.Unlock()
}

func (d *disjuntor) aberto() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.falhas >= d.limite && d.agora().Before(d.abertoAte)
}
