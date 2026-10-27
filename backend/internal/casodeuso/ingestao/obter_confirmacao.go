package ingestao

import (
	"context"

	ding "tccelta/backend/internal/dominio/ingestao"
	"tccelta/backend/internal/dominio/protocolo"
	"tccelta/backend/internal/repositorio"
)

// ObterConfirmacaoEntrada identifica o dongle (já autenticado) que pede o último ACK.
type ObterConfirmacaoEntrada struct{ DevID protocolo.DevID }

// ObterConfirmacao devolve o último ACK gravado, sem chamar o KMS (GET /v1/dongles/{id}/confirmacao).
type ObterConfirmacao struct {
	repos        repositorio.Repositorios
	permitirNone bool
}

// NovoObterConfirmacao liga o caso de uso ao repositório.
func NovoObterConfirmacao(repos repositorio.Repositorios, permitirNone bool) *ObterConfirmacao {
	return &ObterConfirmacao{repos: repos, permitirNone: permitirNone}
}

// Executar devolve o ACK ou ingestao.ErrSemConfirmacao se nunca houve handshake aceito.
func (c *ObterConfirmacao) Executar(ctx context.Context, e ObterConfirmacaoEntrada) ([]byte, error) {
	d, err := c.repos.Dongles.BuscarPorDevID(ctx, e.DevID)
	if err != nil {
		return nil, err
	}
	if err := d.VerificarAcesso(c.permitirNone); err != nil {
		return nil, err
	}
	ack, err := c.repos.Estados.LerUltimoAck(ctx, d.ID)
	if err != nil {
		return nil, err
	}
	if len(ack) == 0 {
		return nil, ding.ErrSemConfirmacao
	}
	return ack, nil
}
