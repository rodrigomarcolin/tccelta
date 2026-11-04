package worker

import (
	"context"
	"errors"
	"sync"
	"time"

	"tccelta/backend/internal/port"
	"tccelta/backend/internal/usecase/tripcalc"
)

// startTrips inicia o fluxo das viagens (spec API1): o avaliador de "os dados chegaram?", as goroutines de cálculo
// (compartilhadas com o dreno de recalc_pending) e o ticker das viagens provisórias. O NOTIFY trip_pending acorda; os
// tickers são a rede de segurança.
func (w *Worker) startTrips(ctx context.Context, wg *sync.WaitGroup) {
	if w.Trips == nil {
		return
	}
	signals := w.Signals.Listen(ctx, "trip_pending", w.Met.Reconnection)
	n := max(1, w.Cfg.Concurrency)
	jobs := make(chan tripcalc.Ready, 4*n)
	evalWake := make(chan struct{}, 1)
	recalcWake := make([]chan struct{}, n)
	var inflight sync.Map // uuid.UUID -> struct{}: evita enfileirar de novo a viagem que ainda está sendo calculada

	wake := func() {
		select {
		case evalWake <- struct{}{}:
		default:
		}
		for _, c := range recalcWake {
			select {
			case c <- struct{}{}:
			default:
			}
		}
	}

	wg.Add(1)
	go func() { // sinais do banco
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-signals:
				if !ok {
					return
				}
				wake()
			}
		}
	}()

	wg.Add(1)
	go func() { // avaliador
		defer wg.Done()
		t := time.NewTicker(w.Cfg.TripEvalInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			case <-evalWake:
			}
			ready, err := w.Trips.Evaluate(ctx)
			if err != nil && ctx.Err() == nil {
				w.Log.Error("worker.viagem_avaliacao_falhou", "erro", err)
			}
			for _, r := range ready {
				if _, busy := inflight.LoadOrStore(r.ID, struct{}{}); busy {
					continue
				}
				select {
				case jobs <- r:
				default:
					inflight.Delete(r.ID) // fila cheia: o próximo ciclo tenta de novo
				}
			}
		}
	}()

	for i := 0; i < n; i++ {
		recalcWake[i] = make(chan struct{}, 1)
		wg.Add(1)
		go func(wake <-chan struct{}) {
			defer wg.Done()
			t := time.NewTicker(w.Cfg.PollInterval)
			defer t.Stop()
			w.drainRecalc(ctx)
			for {
				select {
				case <-ctx.Done():
					return
				case r := <-jobs:
					w.calculate(ctx, r)
					inflight.Delete(r.ID)
				case <-wake:
					w.drainRecalc(ctx)
				case <-t.C:
					w.drainRecalc(ctx)
				}
			}
		}(recalcWake[i])
	}

	wg.Add(1)
	go func() { // viagens provisórias
		defer wg.Done()
		t := time.NewTicker(w.Cfg.TripProvisionalInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
			marked, err := w.Trips.CheckProvisional(ctx)
			if err != nil && ctx.Err() == nil {
				w.Log.Error("worker.viagem_provisorias_falhou", "erro", err)
			}
			if marked > 0 {
				wake()
			}
		}
	}()
}

func (w *Worker) calculate(ctx context.Context, r tripcalc.Ready) {
	if _, err := w.Trips.Calculate(ctx, r.ID, r.ByLimit); err != nil && ctx.Err() == nil {
		if errors.Is(err, port.ErrDatabaseBusy) {
			w.Log.Warn("worker.viagem_adiada", "trip_id", r.ID, "erro", err)
			return
		}
		w.Log.Error("worker.viagem_falhou_calculo", "trip_id", r.ID, "erro", err)
	}
}

// drainRecalc recalcula as viagens com recalc_pending até a fila esvaziar.
func (w *Worker) drainRecalc(ctx context.Context) {
	for ctx.Err() == nil {
		found, err := w.Trips.Recalc(ctx)
		if err != nil {
			if ctx.Err() == nil {
				w.Log.Error("worker.viagem_recalculo_erro", "erro", err)
			}
			return
		}
		if !found {
			return
		}
	}
}

// reportTripQueue atualiza o gauge das filas de viagens (chamado pelo ticker de 15 s).
func (w *Worker) reportTripQueue(ctx context.Context) {
	if w.Trips == nil {
		return
	}
	if q, err := w.Trips.Queue(ctx); err == nil {
		w.Met.TripQueue(q.Waiting, q.Recalc)
	}
}
