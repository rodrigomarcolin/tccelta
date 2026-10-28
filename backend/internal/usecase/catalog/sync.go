// Package catalog mantém o catálogo de PIDs em memória em dia com a tabela pid.
package catalog

import (
	"context"
	"log/slog"
	"time"

	"tccelta/backend/internal/domain/pid"
	"tccelta/backend/internal/port"
)

// Sync recarrega o catálogo a cada sinal de LISTEN pid_changed e a cada 5 min.
type Sync struct {
	pids port.Pids
	cat  *pid.Catalog
	log  *slog.Logger
}

// NewSync liga o caso de uso ao repositório e ao catálogo.
func NewSync(pids port.Pids, cat *pid.Catalog, log *slog.Logger) *Sync {
	return &Sync{pids: pids, cat: cat, log: log}
}

// Reload lê a tabela pid e troca o catálogo.
func (c *Sync) Reload(ctx context.Context) error {
	defs, err := c.pids.List(ctx)
	if err != nil {
		return err
	}
	c.cat.Set(defs)
	if c.log != nil {
		c.log.Info("pids.recarregado", "n", len(defs))
	}
	return nil
}

// Track recarrega a cada sinal do canal e a cada 5 min, até o contexto terminar.
func (c *Sync) Track(ctx context.Context, signals <-chan string) {
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	reload := func() {
		if err := c.Reload(ctx); err != nil && c.log != nil {
			c.log.Error("pids.recarga_falhou", "erro", err)
		}
	}
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-signals:
			if !ok {
				return
			}
			reload()
		case <-t.C:
			reload()
		}
	}
}
