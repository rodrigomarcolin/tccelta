package port

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"tccelta/backend/internal/domain/protocol"
)

// IssuedToken é o resultado de TokenIssuer.Issue.
type IssuedToken struct {
	Token     string
	JTI       uuid.UUID
	ExpiresAt time.Time
}

// Session é o que o token validado autoriza.
type Session struct {
	Dev       protocol.DevID
	CS        uint32
	JTI       uuid.UUID
	ExpiresAt time.Time
}

// TokenIssuer emite o token de sessão do handshake.
type TokenIssuer interface {
	Issue(ctx context.Context, dev protocol.DevID, cs uint32) (IssuedToken, error)
	Validity() time.Duration
}

// TokenValidator valida o token bruto; devolve ErrInvalidToken ou ErrTokenExpired.
type TokenValidator interface {
	Validate(ctx context.Context, raw string) (Session, error)
}

// Erros de validação do token.
var (
	ErrInvalidToken = errors.New("token_invalido")
	ErrTokenExpired = errors.New("token_expirado")
)
