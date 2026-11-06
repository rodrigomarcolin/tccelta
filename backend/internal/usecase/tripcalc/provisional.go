package tripcalc

import (
	"context"

	"github.com/google/uuid"

	"tccelta/backend/internal/domain/trip"
)

// CheckProvisional é o ticker das viagens provisórias (spec API1): marca recalc_pending quando chegaram blocos novos
// dentro da viagem, quando um boot sem hora ganhou conversão ou quando o boot de uma viagem com boot_generation nula
// apareceu; e limpa provisional, sem recalcular, quando há prova de completude e nenhum dado novo. Devolve quantas
// viagens foram marcadas para recálculo.
func (c *Calculator) CheckProvisional(ctx context.Context) (int, error) {
	list, err := c.repos.Trips.ListProvisional(ctx)
	if err != nil {
		return 0, err
	}
	var mark []uuid.UUID
	for _, p := range list {
		act, err := c.checkOne(ctx, p)
		if err != nil {
			return len(mark), err
		}
		switch act {
		case actRecalc:
			mark = append(mark, p.ID)
		case actComplete:
			if err := c.repos.Trips.ClearProvisional(ctx, p.ID); err != nil {
				return len(mark), err
			}
			c.met.TripProvisional("completed")
		}
	}
	if len(mark) == 0 {
		return 0, nil
	}
	if err := c.repos.Trips.MarkRecalc(ctx, mark); err != nil {
		return 0, err
	}
	for _, id := range mark {
		c.met.TripProvisional("recalc_marked")
		c.log.Info("worker.viagem_recalculo_marcado", "trip_id", id)
	}
	return len(mark), c.repos.Trips.Notify(ctx)
}

type action int

const (
	actNone action = iota
	actRecalc
	actComplete
)

func (c *Calculator) checkOne(ctx context.Context, p trip.Provisional) (action, error) {
	r := c.repos
	prog, err := r.States.ReadProgress(ctx, p.DongleID)
	if err != nil {
		return actNone, err
	}
	t := p.Trip
	// entrada boot com boot_generation nula: o boot pode ter chegado
	if t.EntryType == "boot" && t.BootGeneration == nil && t.BootID != nil {
		_, ok, err := r.Trips.ResolveGeneration(ctx, t.ID, t.DongleID, *t.BootID)
		if err != nil || ok {
			return actRecalc, err
		}
		return actNone, nil
	}
	// boot sem hora que ganhou conversão
	for _, ref := range p.NoTimeBoots {
		b, found, err := r.TripSources.FindBoot(ctx, t.DongleID, ref.Generation, ref.BootID)
		if err != nil {
			return actNone, err
		}
		if found && b.HasTime() {
			return actRecalc, nil
		}
	}
	boots, err := c.tripBoots(ctx, t)
	if err != nil {
		return actNone, err
	}
	through := int64(0)
	if t.CalculatedThroughSeq != nil {
		through = *t.CalculatedThroughSeq
	}
	if prog.ContiguousSeq > through {
		for _, b := range boots {
			if b.LastSeq != nil && *b.LastSeq > through {
				return actRecalc, nil
			}
		}
	}
	if len(p.NoTimeBoots) > 0 { // enquanto faltar a hora de um boot, a viagem continua provisória
		return actNone, nil
	}
	proof, err := c.completeProof(ctx, r, t, boots, prog.ContiguousSeq)
	if err != nil {
		return actNone, err
	}
	if proof {
		return actComplete, nil
	}
	return actNone, nil
}

// tripBoots são os boots que a viagem cobre agora: o boot da entrada ou, no intervalo, os convertidos que o tocam.
func (c *Calculator) tripBoots(ctx context.Context, t trip.Trip) ([]trip.Boot, error) {
	if t.EntryType == "interval" {
		return c.repos.TripSources.IntervalBoots(ctx, t.DongleID, float64(t.IntervalStart.UnixMilli())-folgaMs, float64(t.IntervalEnd.UnixMilli())+folgaMs)
	}
	if t.BootGeneration == nil || t.BootID == nil {
		return nil, nil
	}
	b, found, err := c.repos.TripSources.FindBoot(ctx, t.DongleID, *t.BootGeneration, *t.BootID)
	if err != nil || !found {
		return nil, err
	}
	return []trip.Boot{b}, nil
}
