package trip

import (
	"context"

	"github.com/google/uuid"

	"tccelta/backend/internal/port"
)

// DeleteTrip é o caso de uso de DELETE /v1/viagens/{id}.
type DeleteTrip struct{ tx port.Transactor }

// NewDeleteTrip liga o caso de uso ao transactor.
func NewDeleteTrip(tx port.Transactor) *DeleteTrip { return &DeleteTrip{tx: tx} }

// Run exclui a viagem (exclusão lógica) ou devolve trip.ErrNotFound se ela não existe ou já foi excluída. Roda em transação (o transactor aplica o lock_timeout de 3 s) porque o
// cálculo segura a trava da linha: o UPDATE espera até o lock_timeout e então vira port.ErrDatabaseBusy.
func (d *DeleteTrip) Run(ctx context.Context, id uuid.UUID) error {
	return d.tx.InTransaction(ctx, func(ctx context.Context, r port.Repositories) error {
		return r.Trips.Exclude(ctx, id)
	})
}
