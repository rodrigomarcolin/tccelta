// Package http é só transporte: define os endpoints, valida o que é de HTTP (Content-Type, JSON,
// base64, limite de corpo, {id}), chama um caso de uso e serializa a resposta.
package http

import (
	"context"

	"github.com/google/uuid"

	"tccelta/backend/internal/domain/trip"
	"tccelta/backend/internal/port"
	"tccelta/backend/internal/usecase/clienttoken"
	"tccelta/backend/internal/usecase/handshake"
	"tccelta/backend/internal/usecase/ingestion"
	"tccelta/backend/internal/usecase/session"
)

// As interfaces ficam aqui, e não em port, porque dependem dos tipos de entrada e saída dos
// casos de uso (porta não pode importá-los sem criar ciclo). Os testes usam fakes delas.

// RequestChallenge é o caso de uso de POST /v1/dongles/{id}/desafio.
type RequestChallenge interface {
	Run(ctx context.Context, e handshake.IssueChallengeInput) (handshake.IssuedChallenge, error)
}

// RespondChallenge é o caso de uso de POST /v1/dongles/{id}/desafio/resposta.
type RespondChallenge interface {
	Run(ctx context.Context, e handshake.RespondChallengeInput) (handshake.HandshakeResult, error)
}

// ReceiveBatch é o caso de uso de POST /v1/dongles/blocos.
type ReceiveBatch interface {
	Run(ctx context.Context, e ingestion.ReceiveBatchInput) (ingestion.BatchResult, error)
}

// GetConfirmation é o caso de uso de GET /v1/dongles/{id}/confirmacao.
type GetConfirmation interface {
	Run(ctx context.Context, e ingestion.GetConfirmationInput) ([]byte, error)
}

// Authenticate é o caso de uso do middleware Bearer.
type Authenticate interface {
	Run(ctx context.Context, raw string) (session.Authenticated, error)
}

// CreateTrip é o caso de uso de POST /v1/viagens.
type CreateTrip interface {
	Run(ctx context.Context, in trip.Input) (trip.Trip, error)
}

// GetTrip é o caso de uso de GET /v1/viagens/{id}.
type GetTrip interface {
	Run(ctx context.Context, id uuid.UUID) (trip.Trip, error)
}

// GetAnalysis é o caso de uso de GET /v1/viagens/{id}/analise.
type GetAnalysis interface {
	Run(ctx context.Context, id uuid.UUID) (trip.Analysis, error)
}

// IssueToken é o caso de uso de POST /v1/auth/token.
type IssueToken interface {
	Run(ctx context.Context, c clienttoken.Credentials) (clienttoken.Token, error)
}

// AuthenticateClient é o caso de uso do middleware Bearer das rotas de viagem.
type AuthenticateClient interface {
	Run(ctx context.Context, raw string) (port.ClientSession, error)
}

// GetClientKeys é o caso de uso de GET /.well-known/jwks.json.
type GetClientKeys interface {
	Run(ctx context.Context) ([]port.ClientKey, error)
}
