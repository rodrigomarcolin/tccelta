// Package jwt emite e valida os JWT (EdDSA) do handshake.
package jwt

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"tccelta/backend/internal/domain/protocol"
	"tccelta/backend/internal/port"
)

var (
	ErrInvalidToken = port.ErrInvalidToken
	ErrTokenExpired = port.ErrTokenExpired
)

// Issuer monta o JWT e o assina no KMS.
type Issuer struct {
	signer           port.JWTSigner
	issuer, audience string
	validity         time.Duration
	clock            func() time.Time
	version          atomic.Int32
}

// NewIssuer cria o emissor.
func NewIssuer(a port.JWTSigner, issuer, audience string, validity time.Duration) *Issuer {
	return &Issuer{signer: a, issuer: issuer, audience: audience, validity: validity}
}

// SetClock troca o relógio (testes).
func (e *Issuer) SetClock(f func() time.Time) { e.clock = f }

// Validity é a duração dos tokens emitidos.
func (e *Issuer) Validity() time.Duration { return e.validity }

func (e *Issuer) now() time.Time {
	if e.clock != nil {
		return e.clock()
	}
	return time.Now()
}

func seg(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Issue monta header.claims em base64url, assina a signing string e devolve o JWT compacto.
// O kid é "jwt-v<versão devolvida pelo Vault>".
func (e *Issuer) Issue(ctx context.Context, dev protocol.DevID, cs uint32) (port.IssuedToken, error) {
	now := e.now()
	jti := uuid.New()
	exp := now.Add(e.validity)
	claims := map[string]any{
		"iss": e.issuer, "aud": e.audience, "sub": dev.Hex(), "jti": jti.String(),
		"iat": now.Unix(), "nbf": now.Unix(), "exp": exp.Unix(), "cs": cs,
	}
	cl, err := seg(claims)
	if err != nil {
		return port.IssuedToken{}, err
	}
	// O kid depende da versão da chave que assina, e o header entra na signing string: usa a última
	// versão conhecida; se o Vault assinar com outra (rotação), refaz uma vez com a versão certa.
	version := int(e.version.Load())
	if version == 0 {
		ks, err := e.signer.PublicKeys(ctx)
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
		hd, err := seg(map[string]any{"alg": "EdDSA", "typ": "JWT", "kid": "jwt-v" + fmt.Sprint(version)})
		if err != nil {
			return port.IssuedToken{}, err
		}
		sign := hd + "." + cl
		sig, v, err := e.signer.Sign(ctx, []byte(sign))
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
	return port.IssuedToken{}, fmt.Errorf("%w: versão da chave do JWT mudou durante a emissão", port.ErrKMSUnavailable)
}
