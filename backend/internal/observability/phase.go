// Package observability guarda o que atravessa camadas só para telemetria.
package observability

import "context"

// Phase indica de onde vem a chamada ao KMS (rótulo de métrica): fora (pre) ou dentro (tx) da transação.
type Phase string

const (
	PhasePre Phase = "pre"
	PhaseTx  Phase = "tx"
)

type phaseKey struct{}

// WithKMSPhase marca o contexto com a fase da chamada (para a métrica tccelta_kms_chamadas_total).
func WithKMSPhase(ctx context.Context, f Phase) context.Context {
	return context.WithValue(ctx, phaseKey{}, f)
}

// KMSPhaseFrom lê a fase do contexto (padrão: pre).
func KMSPhaseFrom(ctx context.Context) Phase {
	if f, ok := ctx.Value(phaseKey{}).(Phase); ok {
		return f
	}
	return PhasePre
}
