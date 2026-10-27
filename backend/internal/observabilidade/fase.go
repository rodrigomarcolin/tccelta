// Package observabilidade guarda o que atravessa camadas só para telemetria.
package observabilidade

import "context"

// Fase indica de onde vem a chamada ao KMS (rótulo de métrica): fora (pre) ou dentro (tx) da transação.
type Fase string

const (
	FasePre Fase = "pre"
	FaseTx  Fase = "tx"
)

type chaveFase struct{}

// ComFaseKMS marca o contexto com a fase da chamada (para a métrica tccelta_kms_chamadas_total).
func ComFaseKMS(ctx context.Context, f Fase) context.Context {
	return context.WithValue(ctx, chaveFase{}, f)
}

// FaseKMSDe lê a fase do contexto (padrão: pre).
func FaseKMSDe(ctx context.Context) Fase {
	if f, ok := ctx.Value(chaveFase{}).(Fase); ok {
		return f
	}
	return FasePre
}
