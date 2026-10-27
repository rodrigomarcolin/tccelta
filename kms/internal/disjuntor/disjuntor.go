// Package disjuntor é o disjuntor (circuit breaker) das chamadas ao Vault.
package disjuntor

import (
	"sync"
	"time"
)

// Disjuntor: `limite` falhas seguidas abrem por `espera`; depois, meia-abertura com 1 sonda.
type Disjuntor struct {
	mu        sync.Mutex
	falhas    int
	limite    int
	espera    time.Duration
	abertoAte time.Time
	sonda     bool
	agora     func() time.Time
}

// Novo cria o disjuntor (o padrão do serviço é Novo(5, 10*time.Second)).
func Novo(limite int, espera time.Duration) *Disjuntor {
	return &Disjuntor{limite: limite, espera: espera, agora: time.Now}
}

// Permitir diz se a chamada pode seguir.
func (d *Disjuntor) Permitir() bool {
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

// Sucesso fecha o disjuntor.
func (d *Disjuntor) Sucesso() {
	d.mu.Lock()
	d.falhas, d.sonda = 0, false
	d.mu.Unlock()
}

// Falha conta uma falha e abre o disjuntor ao atingir o limite.
func (d *Disjuntor) Falha() {
	d.mu.Lock()
	d.falhas++
	d.sonda = false
	if d.falhas >= d.limite {
		d.abertoAte = d.agora().Add(d.espera)
	}
	d.mu.Unlock()
}

// Aberto diz se o disjuntor está recusando chamadas.
func (d *Disjuntor) Aberto() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.falhas >= d.limite && d.agora().Before(d.abertoAte)
}
