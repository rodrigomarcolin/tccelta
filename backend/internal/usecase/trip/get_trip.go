package trip

import (
	"context"

	"github.com/google/uuid"

	domain "tccelta/backend/internal/domain/trip"
	"tccelta/backend/internal/port"
)

// GetTrip é o caso de uso de GET /v1/viagens/{id}.
type GetTrip struct{ repos port.Repositories }

// NewGetTrip liga o caso de uso ao repositório.
func NewGetTrip(repos port.Repositories) *GetTrip { return &GetTrip{repos: repos} }

// Run devolve a viagem ou trip.ErrNotFound.
func (g *GetTrip) Run(ctx context.Context, id uuid.UUID) (domain.Trip, error) {
	return g.repos.Trips.Find(ctx, id)
}
