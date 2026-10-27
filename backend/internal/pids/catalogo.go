// Package pids mantém o catálogo de PIDs em memória, recarregado por LISTEN pid_alterado.
package pids

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"tccelta/backend/internal/banco/consultas"
)

// Def é a definição de um PID.
type Def struct {
	PID                 uint8
	Nome, Unidade       string
	NBytes              int
	Fator, Deslocamento float64
	Min, Max            *float64
}

// Catalogo é seguro para uso concorrente.
type Catalogo struct {
	q   consultas.Querier
	m   atomic.Pointer[map[uint8]Def]
	log *slog.Logger
}

// Novo cria um catálogo vazio; chame Recarregar antes de usar.
func Novo(q consultas.Querier, log *slog.Logger) *Catalogo { return &Catalogo{q: q, log: log} }

// DeMapa cria um catálogo fixo (testes).
func DeMapa(defs []Def) *Catalogo {
	c := &Catalogo{}
	c.definir(defs)
	return c
}

func (c *Catalogo) definir(defs []Def) {
	m := make(map[uint8]Def, len(defs))
	for _, d := range defs {
		m[d.PID] = d
	}
	c.m.Store(&m)
}

// Obter devolve a definição do PID.
func (c *Catalogo) Obter(pid uint8) (Def, bool) {
	m := c.m.Load()
	if m == nil {
		return Def{}, false
	}
	d, ok := (*m)[pid]
	return d, ok
}

// Carregado diz se há catálogo (readiness).
func (c *Catalogo) Carregado() bool { m := c.m.Load(); return m != nil && len(*m) > 0 }

// Recarregar lê a tabela pid.
func (c *Catalogo) Recarregar(ctx context.Context) error {
	rows, err := c.q.ListarPids(ctx)
	if err != nil {
		return err
	}
	defs := make([]Def, 0, len(rows))
	for _, r := range rows {
		defs = append(defs, Def{PID: uint8(r.Pid), Nome: r.Nome, Unidade: r.Unidade, NBytes: int(r.NBytes),
			Fator: r.Fator, Deslocamento: r.Deslocamento, Min: r.ValorMin, Max: r.ValorMax})
	}
	c.definir(defs)
	if c.log != nil {
		c.log.Info("pids.recarregado", "n", len(defs))
	}
	return nil
}

// Acompanhar recarrega a cada sinal do canal e a cada 5 min.
func (c *Catalogo) Acompanhar(ctx context.Context, sinais <-chan string) {
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	recarregar := func() {
		if err := c.Recarregar(ctx); err != nil && c.log != nil {
			c.log.Error("pids.recarga_falhou", "erro", err)
		}
	}
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-sinais:
			if !ok {
				return
			}
			recarregar()
		case <-t.C:
			recarregar()
		}
	}
}
