package simulator

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"tccelta/backend/internal/port"
)

// tripScenario gera uma viagem (200 blocos com PIDs 0x0C, 0x0D, 0x04 e GPS), cria duas viagens em gold.trip (entrada boot e
// entrada intervalo em torno da hora das âncoras) e espera o worker calculá-las (API1). O papel do banco precisa poder
// inserir em gold.trip (tccelta_owner), como o do Inspector.
func tripScenario(ctx context.Context, o Options) error {
	if o.Seeder == nil {
		return fmt.Errorf("o cenário trip precisa de Options.Seeder")
	}
	v := &verifier{log: o.Log}
	q := o.Inspect
	ch := o.Keys[0]
	d := NewDongle(ch, ConfigDongle{FlashCapacity: 30})
	a := NewApp(o.API, d, client(), o.Report)
	started := time.Now()
	if err := a.Handshake(ctx); err != nil {
		return fmt.Errorf("handshake: %w", err)
	}
	if err := generateAndDrain(ctx, a, 200); err != nil {
		return fmt.Errorf("blocos: %w", err)
	}
	est, err := q.DongleState(ctx, ch.Dev)
	if err != nil {
		return err
	}
	bs := waitForBoots(ctx, q, est.DongleID, 15*time.Second)
	v.ok(len(bs) > 0 && bs[len(bs)-1].TimeSource != nil, "o boot tem hora absoluta antes das viagens")

	end := time.Now().Add(5 * time.Second)
	bootTrip, err := o.Seeder.SeedBootTrip(ctx, port.SeedTrip{Category: "e2e", Driver: "Boot", Fuel: "gasolina_e30", DongleID: est.DongleID,
		BootGeneration: est.BootGeneration, BootID: est.LastBootID, TargetSeq: est.HeadSeq})
	if err != nil {
		return fmt.Errorf("viagem boot: %w", err)
	}
	intervalTrip, err := o.Seeder.SeedIntervalTrip(ctx, port.SeedTrip{Category: "e2e", Driver: "Intervalo", Fuel: "etanol_hidratado", DongleID: est.DongleID,
		Start: started.Add(-5 * time.Second), End: end, TargetSeq: est.HeadSeq})
	if err != nil {
		return fmt.Errorf("viagem intervalo: %w", err)
	}
	for name, id := range map[string]uuid.UUID{"boot": bootTrip, "intervalo": intervalTrip} {
		st := waitTripReady(ctx, q, id, 30*time.Second)
		v.ok(st.Status == "ready", "viagem %s ready (estado %q)", name, st.Status)
		v.ok(st.Parts >= 1, "viagem %s com %d parte(s)", name, st.Parts)
		v.ok(st.DistanceKm > 0 && st.EngineOnS > 0, "viagem %s: distância %.4f km, motor ligado %.1f s", name, st.DistanceKm, st.EngineOnS)
	}
	return v.result()
}

// waitTripReady espera a viagem ficar ready (ou o prazo acabar) e devolve o último estado lido.
func waitTripReady(ctx context.Context, q port.Inspector, id uuid.UUID, maxWait time.Duration) port.TripInspection {
	end := time.Now().Add(maxWait)
	var st port.TripInspection
	for {
		if s, err := q.TripStatus(ctx, id); err == nil {
			st = s
			if st.Status == "ready" || st.Status == "failed" {
				return st
			}
		}
		if time.Now().After(end) || ctx.Err() != nil {
			return st
		}
		time.Sleep(500 * time.Millisecond)
	}
}
