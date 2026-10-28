// Package httpapi é só transporte: define os endpoints, valida o que é de HTTP (Content-Type, JSON,
// base64, limite de corpo, {id}), chama um caso de uso e serializa a resposta.
package httpapi

import (
	"context"

	"tccelta/backend/internal/casodeuso/handshake"
	"tccelta/backend/internal/casodeuso/ingestao"
	"tccelta/backend/internal/casodeuso/sessao"
)

// As interfaces ficam aqui, e não em casodeuso/porta, porque dependem dos tipos de entrada e saída dos
// casos de uso (porta não pode importá-los sem criar ciclo). Os testes usam fakes delas.

// PedirDesafio é o caso de uso de POST /v1/dongles/{id}/desafio.
type PedirDesafio interface {
	Executar(ctx context.Context, e handshake.EmitirDesafioEntrada) (handshake.DesafioEmitido, error)
}

// ResponderDesafio é o caso de uso de POST /v1/dongles/{id}/desafio/resposta.
type ResponderDesafio interface {
	Executar(ctx context.Context, e handshake.ResponderDesafioEntrada) (handshake.ResultadoHandshake, error)
}

// ReceberLote é o caso de uso de POST /v1/dongles/blocos.
type ReceberLote interface {
	Executar(ctx context.Context, e ingestao.ReceberLoteEntrada) (ingestao.ResultadoLote, error)
}

// ObterConfirmacao é o caso de uso de GET /v1/dongles/{id}/confirmacao.
type ObterConfirmacao interface {
	Executar(ctx context.Context, e ingestao.ObterConfirmacaoEntrada) ([]byte, error)
}

// Autenticar é o caso de uso do middleware Bearer.
type Autenticar interface {
	Executar(ctx context.Context, bruto string) (sessao.Autenticada, error)
}
