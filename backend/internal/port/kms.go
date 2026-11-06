package port

import (
	"context"
	"crypto/ed25519"
	"errors"
)

// ErrKMSUnavailable: timeout, 5xx, disjuntor aberto (-> 503).
var ErrKMSUnavailable = errors.New("kms_indisponivel")

// Authenticator são as operações com as chaves por dongle, endereçadas pelo nome da chave no KMS
// (Dongle.AuthKey e Dongle.RecKey).
type Authenticator interface {
	// HMACAuth devolve o HMAC-SHA256 COMPLETO (32 B) de cada entrada com a chave K_auth.
	// Quem chama trunca para 16 B e compara com protocolo.CompareTag.
	HMACAuth(ctx context.Context, key string, inputs [][]byte) ([][]byte, error)
	// VerifyBlocks verifica tags de 32 B com a chave K_rec, numa única chamada em lote.
	VerifyBlocks(ctx context.Context, key string, inputs, tags [][]byte) ([]bool, error)
}

// JWTSigner: chave Ed25519 do JWT.
type JWTSigner interface {
	Sign(ctx context.Context, input []byte) (signature []byte, version int, err error)
	PublicKeys(ctx context.Context) (map[int]ed25519.PublicKey, error)
}

// ClientVerifier confere o segredo de um cliente (client credentials) no KMS, que guarda só o hash. O KMS responde
// false para segredo errado, cliente desconhecido e cliente revogado.
type ClientVerifier interface {
	VerifyClient(ctx context.Context, clientID, secret string) (valid bool, err error)
}

// ClientSigner: chave Ed25519 `cli` dos tokens de cliente, separada da do JWT do app.
type ClientSigner interface {
	SignClient(ctx context.Context, input []byte) (signature []byte, version int, err error)
	ClientPublicKeys(ctx context.Context) (map[int]ed25519.PublicKey, error)
}
