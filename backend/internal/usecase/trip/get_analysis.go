package trip

import (
	"context"

	"github.com/google/uuid"

	domain "tccelta/backend/internal/domain/trip"
	"tccelta/backend/internal/port"
)

// GetAnalysis é o caso de uso de GET /v1/viagens/{id}/analise.
type GetAnalysis struct{ repos port.Repositories }

// NewGetAnalysis liga o caso de uso ao repositório.
func NewGetAnalysis(repos port.Repositories) *GetAnalysis { return &GetAnalysis{repos: repos} }

// Run lê a análise gravada pelo worker, sem recalcular. Devolve trip.ErrNotFound se a viagem não existe e
// trip.NotReadyError (que casa com trip.ErrNotReady) se ela ainda não está pronta ou falhou.
func (g *GetAnalysis) Run(ctx context.Context, id uuid.UUID) (domain.Analysis, error) {
	a, err := g.repos.Trips.Analysis(ctx, id)
	if err != nil {
		return domain.Analysis{}, err
	}
	if a.Trip.Status != domain.StatusReady {
		return domain.Analysis{}, domain.NotReadyError{Status: a.Trip.Status}
	}
	return a, nil
}
