// Package token emite e valida os JWT (EdDSA) do handshake.
package token

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"tccelta/backend/internal/casodeuso/porta"
	"tccelta/backend/internal/dominio/protocolo"
)

var (
	ErrTokenInvalido = porta.ErrTokenInvalido
	ErrTokenExpirado = porta.ErrTokenExpirado
)

// Emissor monta o JWT e o assina no KMS.
type Emissor struct {
	assinador          porta.AssinadorJWT
	emissor, audiencia string
	validade           time.Duration
	relogio            func() time.Time
	versao             atomic.Int32
}

// NovoEmissor cria o emissor.
func NovoEmissor(a porta.AssinadorJWT, emissor, audiencia string, validade time.Duration) *Emissor {
	return &Emissor{assinador: a, emissor: emissor, audiencia: audiencia, validade: validade}
}

// DefinirRelogio troca o relógio (testes).
func (e *Emissor) DefinirRelogio(f func() time.Time) { e.relogio = f }

// Validade é a duração dos tokens emitidos.
func (e *Emissor) Validade() time.Duration { return e.validade }

func (e *Emissor) agora() time.Time {
	if e.relogio != nil {
		return e.relogio()
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
func (e *Emissor) Emitir(ctx context.Context, dev protocolo.DevID, cs uint32) (porta.TokenEmitido, error) {
	now := e.agora()
	jti := uuid.New()
	exp := now.Add(e.validade)
	claims := map[string]any{
		"iss": e.emissor, "aud": e.audiencia, "sub": dev.Hex(), "jti": jti.String(),
		"iat": now.Unix(), "nbf": now.Unix(), "exp": exp.Unix(), "cs": cs,
	}
	cl, err := seg(claims)
	if err != nil {
		return porta.TokenEmitido{}, err
	}
	// O kid depende da versão da chave que assina, e o header entra na signing string: usa a última
	// versão conhecida; se o Vault assinar com outra (rotação), refaz uma vez com a versão certa.
	versao := int(e.versao.Load())
	if versao == 0 {
		ks, err := e.assinador.ChavesPublicas(ctx)
		if err != nil {
			return porta.TokenEmitido{}, err
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
			return porta.TokenEmitido{}, err
		}
		assinar := hd + "." + cl
		sig, v, err := e.assinador.Assinar(ctx, []byte(assinar))
		if err != nil {
			return porta.TokenEmitido{}, err
		}
		e.versao.Store(int32(v))
		if v != versao {
			versao = v
			continue
		}
		return porta.TokenEmitido{Token: assinar + "." + base64.RawURLEncoding.EncodeToString(sig), JTI: jti, ExpiraEm: exp}, nil
	}
	return porta.TokenEmitido{}, fmt.Errorf("%w: versão da chave do JWT mudou durante a emissão", porta.ErrKMSIndisponivel)
}
