package trip

import (
	"context"

	domain "tccelta/backend/internal/domain/trip"
	"tccelta/backend/internal/port"
)

// ListTrips é o caso de uso de GET /v1/viagens.
type ListTrips struct{ repos port.Repositories }

// NewListTrips liga o caso de uso ao repositório.
func NewListTrips(repos port.Repositories) *ListTrips { return &ListTrips{repos: repos} }

// Run devolve a página pedida; a query já vem validada (domain.NewListQuery).
func (l *ListTrips) Run(ctx context.Context, q domain.ListQuery) (domain.Page, error) {
	return l.repos.Trips.List(ctx, q)
}
