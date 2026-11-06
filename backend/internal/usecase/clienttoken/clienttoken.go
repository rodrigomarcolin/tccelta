// Package clienttoken tem os casos de uso do client credentials: emitir o token (POST /v1/auth/token), validar o Bearer das
// rotas de viagem e listar as chaves públicas (JWKS).
package clienttoken

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"tccelta/backend/internal/domain/clientauth"
	"tccelta/backend/internal/port"
)

// Credentials é o que o transporte extraiu do pedido de token.
type Credentials struct {
	// Present: o cabeçalho Authorization: Basic veio e foi decodificado.
	Present          bool
	ClientID, Secret string
	IP               string // IP do cliente já normalizado (IPv6 agrupado por /64)
}

// Token é a resposta de sucesso do endpoint de token.
type Token struct {
	AccessToken string
	ExpiresIn   time.Duration
}

// IssueToken verifica o segredo no KMS e emite o JWT, com o bloqueio progressivo na frente.
type IssueToken struct {
	verifier port.ClientVerifier
	issuer   port.ClientTokenIssuer
	blocker  port.AuthBlocker
	met      port.MetricsAuth
	log      *slog.Logger
}

// NewIssueToken liga o caso de uso às suas portas. met pode ser nil.
func NewIssueToken(v port.ClientVerifier, i port.ClientTokenIssuer, b port.AuthBlocker, met port.MetricsAuth, log *slog.Logger) *IssueToken {
	return &IssueToken{verifier: v, issuer: i, blocker: b, met: met, log: log}
}

// Run devolve clientauth.BlockedError (bloqueio ativo; o KMS não é chamado), clientauth.ErrInvalidClient ou
// port.ErrKMSUnavailable (que não conta como falha).
func (u *IssueToken) Run(ctx context.Context, c Credentials) (Token, error) {
	valid := c.Present && clientauth.ValidID(c.ClientID) && clientauth.ValidSecret(c.Secret)
	pair := ""
	if valid {
		pair = c.ClientID + "|" + c.IP
	}
	if wait := u.blocker.Check(ctx, pair, c.IP); wait > 0 {
		return Token{}, clientauth.BlockedError{RetryAfter: wait}
	}
	if !valid {
		// sem Basic ou client_id/segredo malformado: só o contador do IP
		u.fail(ctx, "", c)
		return Token{}, clientauth.ErrInvalidClient
	}
	ok, err := u.verifier.VerifyClient(ctx, c.ClientID, c.Secret)
	if err != nil {
		return Token{}, err
	}
	if !ok {
		u.fail(ctx, pair, c)
		return Token{}, clientauth.ErrInvalidClient
	}
	u.blocker.Reset(ctx, pair)
	t, err := u.issuer.Issue(ctx, c.ClientID)
	if err != nil {
		return Token{}, err
	}
	return Token{AccessToken: t.Token, ExpiresIn: u.issuer.Validity()}, nil
}

// fail registra a falha e conta os bloqueios novos. O client_id só vai ao log se casar com o formato; o segredo, nunca.
func (u *IssueToken) fail(ctx context.Context, pair string, c Credentials) {
	o := u.blocker.Fail(ctx, pair, c.IP)
	if u.met != nil {
		if o.PairBlock > 0 {
			u.met.Block("pair")
		}
		if o.IPBlock > 0 {
			u.met.Block("ip")
		}
	}
	id := ""
	if clientauth.ValidID(c.ClientID) {
		id = c.ClientID
	}
	u.log.Warn("auth.falha", "client_id", id, "ip", c.IP, "n_par", o.PairCount, "n_ip", o.IPCount,
		"bloqueio_par_s", int(o.PairBlock.Seconds()), "bloqueio_ip_s", int(o.IPBlock.Seconds()))
}

// Authenticate valida o Bearer das rotas de viagem.
type Authenticate struct{ validator port.ClientTokenValidator }

// NewAuthenticate liga o caso de uso ao validador.
func NewAuthenticate(v port.ClientTokenValidator) *Authenticate { return &Authenticate{validator: v} }

// Run devolve port.ErrInvalidToken ou port.ErrTokenExpired para token ruim.
func (a *Authenticate) Run(ctx context.Context, raw string) (port.ClientSession, error) {
	return a.validator.Validate(ctx, raw)
}

// ErrNoKeys: nenhuma chave pública carregada (o JWKS responde 503).
var ErrNoKeys = errors.New("sem_chaves")

// GetKeys lista as chaves públicas dos tokens de cliente.
type GetKeys struct{ keys port.ClientKeySet }

// NewGetKeys liga o caso de uso ao cache de chaves do validador.
func NewGetKeys(k port.ClientKeySet) *GetKeys { return &GetKeys{keys: k} }

// Run devolve as chaves em memória ou ErrNoKeys.
func (g *GetKeys) Run(context.Context) ([]port.ClientKey, error) {
	ks := g.keys.ClientKeys()
	if len(ks) == 0 {
		return nil, ErrNoKeys
	}
	return ks, nil
}
