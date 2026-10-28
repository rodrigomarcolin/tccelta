package porta

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"tccelta/backend/internal/dominio/protocolo"
)

// TokenEmitido é o resultado de EmissorToken.Emitir.
type TokenEmitido struct {
	Token    string
	JTI      uuid.UUID
	ExpiraEm time.Time
}

// Sessao é o que o token validado autoriza.
type Sessao struct {
	Dev      protocolo.DevID
	CS       uint32
	JTI      uuid.UUID
	ExpiraEm time.Time
}

// EmissorToken emite o token de sessão do handshake.
type EmissorToken interface {
	Emitir(ctx context.Context, dev protocolo.DevID, cs uint32) (TokenEmitido, error)
	Validade() time.Duration
}

// ValidadorToken valida o token bruto; devolve ErrTokenInvalido ou ErrTokenExpirado.
type ValidadorToken interface {
	Validar(ctx context.Context, bruto string) (Sessao, error)
}

// Erros de validação do token.
var (
	ErrTokenInvalido = errors.New("token_invalido")
	ErrTokenExpirado = errors.New("token_expirado")
)
