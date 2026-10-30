// Package provisioning cadastra no banco os dongles que o serviço KMS já provisionou. Depende só do domínio e das
// portas; a leitura e a validação do record.json ficam na ferramenta (internal/tools/provisioning).
package provisioning

import (
	"context"
	"errors"
	"log/slog"

	"tccelta/backend/internal/domain/dongle"
	"tccelta/backend/internal/port"
)

// Registrar cadastra dongles provisionados.
type Registrar struct {
	repos port.Repositories // fora de transação
	tx    port.Transactor
	log   *slog.Logger
}

// New liga o caso de uso às portas.
func New(repos port.Repositories, tx port.Transactor, log *slog.Logger) *Registrar {
	return &Registrar{repos: repos, tx: tx, log: log}
}

// Register insere dongle e dongle_state numa transação. Idempotente: DevId já cadastrado é pulado
// (created = false).
func (c *Registrar) Register(ctx context.Context, g dongle.Registration) (created bool, err error) {
	if _, err := c.repos.Dongles.FindByDevID(ctx, g.DevID); err == nil {
		c.log.Info("provisionamento.ja_cadastrado", "dev_id", g.DevID.Hex())
		return false, nil
	} else if !errors.Is(err, dongle.ErrUnknown) {
		return false, err
	}
	err = c.tx.InTransaction(ctx, func(ctx context.Context, r port.Repositories) error {
		_, err := r.Dongles.Register(ctx, g)
		return err
	})
	if err != nil {
		return false, err
	}
	c.log.Info("provisionamento.cadastrado", "dev_id", g.DevID.Hex(), "perfil", g.Profile)
	return true, nil
}
