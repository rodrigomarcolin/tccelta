// Package trip tem os casos de uso das rotas de viagem (spec API2): criar a viagem, ler o estado e ler a análise. A API só
// valida, congela o ponto de corte dos dados e lê a gold; o cálculo é do worker (usecase/tripcalc).
package trip

import (
	"context"
	"errors"

	domain "tccelta/backend/internal/domain/trip"
	"tccelta/backend/internal/port"
)

// CreateTrip é o caso de uso de POST /v1/viagens.
type CreateTrip struct{ tx port.Transactor }

// NewCreateTrip liga o caso de uso ao transactor.
func NewCreateTrip(tx port.Transactor) *CreateTrip { return &CreateTrip{tx: tx} }

// maxCreateAttempts limita as tentativas quando o índice único recusa o Insert mas a viagem que o causou já não bloqueia (falhou
// ou foi excluída entre as duas leituras); na prática basta a segunda.
const maxCreateAttempts = 3

// Run valida a entrada e, numa só transação, lê o ponto de corte (head_seq do dongle), confere e resolve a geração do boot, procura
// a mesma viagem, grava a viagem em pending e envia o NOTIFY trip_pending (que só sai no commit). Devolve dongle.ErrUnknown se o
// dongle não tem dongle_state, trip.ErrBootNotFound se a entrada é um boot que nunca chegou, trip.RepeatedTripError (ErrTripRepeated)
// se a mesma entrada já foi processada, de qualquer motorista, e trip.ErrDriverRepeated se o motorista já tem viagem na categoria.
func (c *CreateTrip) Run(ctx context.Context, in domain.Input) (domain.Trip, error) {
	draft, err := domain.NewDraft(in)
	if err != nil {
		return domain.Trip{}, err
	}
	for attempt := 1; ; attempt++ {
		created, resolved, err := c.create(ctx, draft)
		var repeated domain.RepeatedTripError
		if !errors.Is(err, domain.ErrTripRepeated) || errors.As(err, &repeated) {
			return created, err
		}
		// Corrida: o índice único recusou o Insert depois de o FindSame não achar nada. A transação abortou, então a viagem que
		// ganhou é lida em outra.
		var same domain.Trip
		var found bool
		if err := c.tx.InTransaction(ctx, func(ctx context.Context, r port.Repositories) (err error) {
			same, found, err = r.Trips.FindSame(ctx, resolved)
			return err
		}); err != nil {
			return domain.Trip{}, err
		}
		if found {
			return domain.Trip{}, domain.RepeatedTripError{ID: same.ID, DriverName: same.DriverName}
		}
		if attempt >= maxCreateAttempts {
			return domain.Trip{}, err
		}
	}
}

// create é uma tentativa de criar a viagem. Devolve também o rascunho com a geração resolvida, para o FindSame da corrida.
func (c *CreateTrip) create(ctx context.Context, draft domain.Draft) (domain.Trip, domain.Draft, error) {
	var created domain.Trip
	resolved := draft
	err := c.tx.InTransaction(ctx, func(ctx context.Context, r port.Repositories) error {
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
		resolved = d
		// A mesma viagem vale antes do nome do motorista: quem repete a entrada vai para a viagem que já existe.
		if same, found, err := r.Trips.FindSame(ctx, d); err != nil {
			return err
		} else if found {
			return domain.RepeatedTripError{ID: same.ID, DriverName: same.DriverName}
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
	return created, resolved, err
}
