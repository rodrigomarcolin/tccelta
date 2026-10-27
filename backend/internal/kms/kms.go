// Package kms define as interfaces do KMS (Vault Transit) e a implementação Vault.
package kms

import (
	"context"
	"crypto/ed25519"
	"errors"

	"tccelta/backend/internal/dongles"
)

// ErrIndisponivel: timeout, 5xx, disjuntor aberto, token inválido (-> 503).
var ErrIndisponivel = errors.New("kms_indisponivel")

// Autenticador: operações com as chaves por dongle.
type Autenticador interface {
	// HMACAuth devolve o HMAC-SHA256 COMPLETO (32 B) de cada entrada com K_auth do dongle.
	// Quem chama trunca para 16 B e compara com protocolo.CompararTag.
	HMACAuth(ctx context.Context, d dongles.Dongle, entradas [][]byte) ([][]byte, error)
	// VerificarBlocos verifica tags de 32 B com K_rec; uma chamada com batch_input.
	VerificarBlocos(ctx context.Context, d dongles.Dongle, entradas, tags [][]byte) ([]bool, error)
}

// AssinadorJWT: chave Ed25519 no Transit.
type AssinadorJWT interface {
	Assinar(ctx context.Context, entrada []byte) (assinatura []byte, versao int, err error)
	ChavesPublicas(ctx context.Context) (map[int]ed25519.PublicKey, error)
}

// Fase indica de onde vem a chamada (rótulo de métrica): fora (pre) ou dentro (tx) da transação.
type Fase string

const (
	FasePre Fase = "pre"
	FaseTx  Fase = "tx"
)

type chaveFase struct{}

// ComFase marca o contexto com a fase da chamada (para a métrica tccelta_kms_chamadas_total).
func ComFase(ctx context.Context, f Fase) context.Context {
	return context.WithValue(ctx, chaveFase{}, f)
}

// FaseDe lê a fase do contexto (padrão: pre).
func FaseDe(ctx context.Context) Fase {
	if f, ok := ctx.Value(chaveFase{}).(Fase); ok {
		return f
	}
	return FasePre
}
