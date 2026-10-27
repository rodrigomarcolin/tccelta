// Package porta reúne as portas compartilhadas pelos casos de uso (tudo que não é banco).
package porta

import (
	"context"
	"crypto/ed25519"
	"errors"
)

// ErrKMSIndisponivel: timeout, 5xx, disjuntor aberto (-> 503).
var ErrKMSIndisponivel = errors.New("kms_indisponivel")

// Autenticador são as operações com as chaves por dongle, endereçadas pelo nome da chave no KMS
// (Dongle.ChaveAuth e Dongle.ChaveRec).
type Autenticador interface {
	// HMACAuth devolve o HMAC-SHA256 COMPLETO (32 B) de cada entrada com a chave K_auth.
	// Quem chama trunca para 16 B e compara com protocolo.CompararTag.
	HMACAuth(ctx context.Context, chave string, entradas [][]byte) ([][]byte, error)
	// VerificarBlocos verifica tags de 32 B com a chave K_rec, numa única chamada em lote.
	VerificarBlocos(ctx context.Context, chave string, entradas, tags [][]byte) ([]bool, error)
}

// AssinadorJWT: chave Ed25519 do JWT.
type AssinadorJWT interface {
	Assinar(ctx context.Context, entrada []byte) (assinatura []byte, versao int, err error)
	ChavesPublicas(ctx context.Context) (map[int]ed25519.PublicKey, error)
}
