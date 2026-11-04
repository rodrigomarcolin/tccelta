package port

import (
	"context"
	"crypto/ed25519"
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

// ClientSession é o que o token de cliente validado autoriza.
type ClientSession struct {
	ClientID  string
	JTI       uuid.UUID
	ExpiresAt time.Time
}

// ClientTokenIssuer emite o token do client credentials para o client_id já verificado.
type ClientTokenIssuer interface {
	Issue(ctx context.Context, clientID string) (IssuedToken, error)
	Validity() time.Duration
}

// ClientTokenValidator valida o token de cliente bruto; devolve ErrInvalidToken ou ErrTokenExpired.
type ClientTokenValidator interface {
	Validate(ctx context.Context, raw string) (ClientSession, error)
}

// ClientKey é uma chave pública Ed25519 dos tokens de cliente, com o kid ("cli-v<n>").
type ClientKey struct {
	KID string
	Key ed25519.PublicKey
}

// ClientKeySet dá as chaves públicas vigentes dos tokens de cliente, as mesmas que o validador usa (para o JWKS).
type ClientKeySet interface {
	// ClientKeys devolve as chaves em memória, ordenadas pelo kid; vazio se nenhuma foi carregada.
	ClientKeys() []ClientKey
}

// AuthBlockOutcome é o resultado de registrar uma falha de segredo.
type AuthBlockOutcome struct {
	PairCount, IPCount int
	// PairBlock e IPBlock são o bloqueio novo causado por esta falha em cada contador (zero se nenhum).
	PairBlock, IPBlock time.Duration
}

// AuthBlocker guarda os contadores do bloqueio progressivo de POST /v1/auth/token: um por par (client_id, IP) e um por IP.
// As implementações falham aberto: erro do armazenamento nunca vira bloqueio.
type AuthBlocker interface {
	// Check devolve o maior tempo de bloqueio restante entre o par e o IP; zero se livre. pair vazio consulta só o IP.
	Check(ctx context.Context, pair, ip string) time.Duration
	// Fail registra uma falha no par (se não vazio) e no IP.
	Fail(ctx context.Context, pair, ip string) AuthBlockOutcome
	// Reset zera só o contador do par; o do IP segue.
	Reset(ctx context.Context, pair string)
}

// MetricsAuth são os eventos do client credentials.
type MetricsAuth interface {
	// TokenRequest conta uma requisição de token: issued, invalid_client, blocked, invalid_request ou kms_unavailable.
	TokenRequest(result string)
	// Block conta um bloqueio novo: pair ou ip.
	Block(scope string)
	// BlockStoreFailure conta uma falha do armazenamento do bloqueio (Redis), coberta pelo estado local.
	BlockStoreFailure()
}
