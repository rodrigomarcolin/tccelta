// Package token emite e valida os JWT (EdDSA) do handshake.
package token

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"tccelta/backend/internal/kms"
	"tccelta/backend/internal/protocolo"
)

var (
	ErrTokenInvalido = errors.New("token_invalido")
	ErrTokenExpirado = errors.New("token_expirado")
)

// Emissor monta o JWT e o assina no Transit.
type Emissor struct {
	Assinador          kms.AssinadorJWT
	Emissor, Audiencia string
	Validade           time.Duration
	Agora              func() time.Time
	versao             atomic.Int32
}

// Emitido é o resultado de Emitir.
type Emitido struct {
	Token    string
	JTI      uuid.UUID
	ExpiraEm time.Time
}

func (e *Emissor) agora() time.Time {
	if e.Agora != nil {
		return e.Agora()
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

// Emitir monta header.claims em base64url, assina a signing string e devolve o JWT compacto.
// O kid é "jwt-v<versão devolvida pelo Vault>".
func (e *Emissor) Emitir(ctx context.Context, dev protocolo.DevID, cs uint32) (Emitido, error) {
	now := e.agora()
	jti := uuid.New()
	exp := now.Add(e.Validade)
	claims := map[string]any{
		"iss": e.Emissor, "aud": e.Audiencia, "sub": dev.Hex(), "jti": jti.String(),
		"iat": now.Unix(), "nbf": now.Unix(), "exp": exp.Unix(), "cs": cs,
	}
	cl, err := seg(claims)
	if err != nil {
		return Emitido{}, err
	}
	// O kid depende da versão da chave que assina, e o header entra na signing string: usa a última
	// versão conhecida; se o Vault assinar com outra (rotação), refaz uma vez com a versão certa.
	versao := int(e.versao.Load())
	if versao == 0 {
		ks, err := e.Assinador.ChavesPublicas(ctx)
		if err != nil {
			return Emitido{}, err
		}
		for k := range ks {
			if k > versao {
				versao = k
			}
		}
	}
	for tentativa := 0; tentativa < 2; tentativa++ {
		hd, err := seg(map[string]any{"alg": "EdDSA", "typ": "JWT", "kid": "jwt-v" + fmt.Sprint(versao)})
		if err != nil {
			return Emitido{}, err
		}
		assinar := hd + "." + cl
		sig, v, err := e.Assinador.Assinar(ctx, []byte(assinar))
		if err != nil {
			return Emitido{}, err
		}
		e.versao.Store(int32(v))
		if v != versao {
			versao = v
			continue
		}
		return Emitido{Token: assinar + "." + base64.RawURLEncoding.EncodeToString(sig), JTI: jti, ExpiraEm: exp}, nil
	}
	return Emitido{}, fmt.Errorf("%w: versão da chave do JWT mudou durante a emissão", kms.ErrIndisponivel)
}
