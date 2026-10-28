// Package catalogo mantém o catálogo de PIDs em memória em dia com a tabela pid.
package catalogo

import (
	"context"
	"log/slog"
	"time"

	"tccelta/backend/internal/dominio/pid"
	"tccelta/backend/internal/repositorio"
)

// Sincronizar recarrega o catálogo a cada sinal de LISTEN pid_alterado e a cada 5 min.
type Sincronizar struct {
	pids repositorio.Pids
	cat  *pid.Catalogo
	log  *slog.Logger
}

// NovoSincronizar liga o caso de uso ao repositório e ao catálogo.
func NovoSincronizar(pids repositorio.Pids, cat *pid.Catalogo, log *slog.Logger) *Sincronizar {
	return &Sincronizar{pids: pids, cat: cat, log: log}
}

// Recarregar lê a tabela pid e troca o catálogo.
func (c *Sincronizar) Recarregar(ctx context.Context) error {
	defs, err := c.pids.Listar(ctx)
	if err != nil {
		return err
	}
	c.cat.Definir(defs)
	if c.log != nil {
		c.log.Info("pids.recarregado", "n", len(defs))
	}
	return nil
}

// Acompanhar recarrega a cada sinal do canal e a cada 5 min, até o contexto terminar.
func (c *Sincronizar) Acompanhar(ctx context.Context, sinais <-chan string) {
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
