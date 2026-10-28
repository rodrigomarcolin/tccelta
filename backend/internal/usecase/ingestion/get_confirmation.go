package ingestion

import (
	"context"

	ding "tccelta/backend/internal/domain/ingestion"
	"tccelta/backend/internal/domain/protocol"
	"tccelta/backend/internal/port"
)

// GetConfirmationInput identifica o dongle (já autenticado) que pede o último ACK.
type GetConfirmationInput struct{ DevID protocol.DevID }

// GetConfirmation devolve o último ACK gravado, sem chamar o KMS (GET /v1/dongles/{id}/confirmacao).
type GetConfirmation struct {
	repos     port.Repositories
	allowNone bool
}

// NewGetConfirmation liga o caso de uso ao repositório.
func NewGetConfirmation(repos port.Repositories, allowNone bool) *GetConfirmation {
	return &GetConfirmation{repos: repos, allowNone: allowNone}
}

// Run devolve o ACK ou ingestao.ErrNoConfirmation se nunca houve handshake aceito.
func (c *GetConfirmation) Run(ctx context.Context, e GetConfirmationInput) ([]byte, error) {
	d, err := c.repos.Dongles.FindByDevID(ctx, e.DevID)
	if err != nil {
		return nil, err
	}
	if err := d.VerifyAccess(c.allowNone); err != nil {
		return nil, err
	}
	ack, err := c.repos.States.ReadLastAck(ctx, d.ID)
	if err != nil {
		return nil, err
	}
	if len(ack) == 0 {
		return nil, ding.ErrNoConfirmation
	}
	return ack, nil
}
