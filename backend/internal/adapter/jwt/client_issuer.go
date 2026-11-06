package jwt

import (
	"context"
	"encoding/base64"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"tccelta/backend/internal/port"
)

// ClientIssuer monta o JWT do client credentials e o assina no KMS com a chave `cli`. Repete a lógica de Issuer
// (kid pela versão que o KMS devolveu, refazendo uma vez se ela mudar), com claims próprias: sem `cs`, `sub` é o client_id.
type ClientIssuer struct {
	signer           port.ClientSigner
	issuer, audience string
	validity         time.Duration
	clock            func() time.Time
	version          atomic.Int32
}

// NewClientIssuer cria o emissor.
func NewClientIssuer(s port.ClientSigner, issuer, audience string, validity time.Duration) *ClientIssuer {
	return &ClientIssuer{signer: s, issuer: issuer, audience: audience, validity: validity}
}

// SetClock troca o relógio (testes).
func (e *ClientIssuer) SetClock(f func() time.Time) { e.clock = f }

// Validity é a duração dos tokens emitidos.
func (e *ClientIssuer) Validity() time.Duration { return e.validity }

func (e *ClientIssuer) now() time.Time {
	if e.clock != nil {
		return e.clock()
	}
	return time.Now()
}

// Issue monta header.claims em base64url, assina a signing string e devolve o JWT compacto. O kid é "cli-v<versão>".
func (e *ClientIssuer) Issue(ctx context.Context, clientID string) (port.IssuedToken, error) {
	now := e.now()
	jti := uuid.New()
	exp := now.Add(e.validity)
	cl, err := seg(map[string]any{
		"iss": e.issuer, "aud": e.audience, "sub": clientID, "jti": jti.String(),
		"iat": now.Unix(), "nbf": now.Unix(), "exp": exp.Unix(),
	})
	if err != nil {
		return port.IssuedToken{}, err
	}
	version := int(e.version.Load())
	if version == 0 {
		ks, err := e.signer.ClientPublicKeys(ctx)
		if err != nil {
			return port.IssuedToken{}, err
		}
		for k := range ks {
			if k > version {
				version = k
			}
		}
	}
	for attempt := 0; attempt < 2; attempt++ {
		hd, err := seg(map[string]any{"alg": "EdDSA", "typ": "JWT", "kid": ClientKIDPrefix + fmt.Sprint(version)})
		if err != nil {
			return port.IssuedToken{}, err
		}
		sign := hd + "." + cl
		sig, v, err := e.signer.SignClient(ctx, []byte(sign))
		if err != nil {
			return port.IssuedToken{}, err
		}
		e.version.Store(int32(v))
		if v != version {
			version = v
			continue
		}
		return port.IssuedToken{Token: sign + "." + base64.RawURLEncoding.EncodeToString(sig), JTI: jti, ExpiresAt: exp}, nil
	}
	return port.IssuedToken{}, fmt.Errorf("%w: versão da chave do JWT de cliente mudou durante a emissão", port.ErrKMSUnavailable)
}

var _ port.ClientTokenIssuer = (*ClientIssuer)(nil)
