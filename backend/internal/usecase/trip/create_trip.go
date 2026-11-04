// Package trip tem os casos de uso das rotas de viagem (spec API2): criar a viagem, ler o estado e ler a análise. A API só
// valida, congela o ponto de corte dos dados e lê a gold; o cálculo é do worker (usecase/tripcalc).
package trip

import (
	"context"

	domain "tccelta/backend/internal/domain/trip"
	"tccelta/backend/internal/port"
)

// CreateTrip é o caso de uso de POST /v1/viagens.
type CreateTrip struct{ tx port.Transactor }

// NewCreateTrip liga o caso de uso ao transactor.
func NewCreateTrip(tx port.Transactor) *CreateTrip { return &CreateTrip{tx: tx} }

// Run valida a entrada e, numa só transação, lê o ponto de corte (head_seq do dongle), confere e resolve a geração do boot, grava a
// viagem em pending e envia o NOTIFY trip_pending (que só sai no commit). Devolve dongle.ErrUnknown se o dongle não tem
// dongle_state, trip.ErrBootNotFound se a entrada é um boot que nunca chegou e trip.ErrDriverRepeated se o motorista já tem
// viagem na categoria.
func (c *CreateTrip) Run(ctx context.Context, in domain.Input) (domain.Trip, error) {
	draft, err := domain.NewDraft(in)
	if err != nil {
		return domain.Trip{}, err
	}
	var created domain.Trip
	err = c.tx.InTransaction(ctx, func(ctx context.Context, r port.Repositories) error {
		d := draft // a transação pode ser repetida: parte sempre da entrada validada
		st, err := r.States.Read(ctx, d.DongleID)
		if err != nil {
			return err
		}
		d.TargetSeq = st.HeadSeq
		if d.EntryType == domain.EntryTypeBoot {
			// O boot precisa existir: sem nenhuma geração dele a viagem nasceria vazia e ficaria à espera de dados que talvez
			// nunca cheguem. Sem geração informada vale a maior que tem o boot.
			g, found, err := r.Boots.LatestGeneration(ctx, d.DongleID, *d.BootID)
			if err != nil {
				return err
			}
			if !found {
				return domain.ErrBootNotFound
			}
			if d.BootGeneration == nil {
				d.BootGeneration = &g
			}
		}
		t, err := r.Trips.Insert(ctx, d)
		if err != nil {
			return err
		}
		if err := r.Trips.NotifyTrip(ctx, t.ID); err != nil {
			return err
		}
		created = t
		return nil
	})
	return created, err
}
