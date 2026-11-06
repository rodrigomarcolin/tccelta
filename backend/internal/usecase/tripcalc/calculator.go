// Package tripcalc é o caso de uso do worker de viagens (spec API1): decide quando os dados de uma viagem já chegaram,
// carrega a silver no eixo de tempo da análise, chama domain/tripanalysis e grava o resultado na gold. Também reavalia as
// viagens provisórias quando chegam dados atrasados. Depende só do domínio e das portas; o agendamento (LISTEN, tickers,
// concorrência) fica em internal/worker.
package tripcalc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"tccelta/backend/internal/domain/timeline"
	"tccelta/backend/internal/domain/trip"
	"tccelta/backend/internal/domain/tripanalysis"
	"tccelta/backend/internal/port"
)

// Config são os parâmetros do worker de viagens.
type Config struct {
	// WaitNoBatch é o silêncio de lotes que libera o cálculo (VIAGEM_ESPERA_SEM_LOTE).
	WaitNoBatch time.Duration
	// FinalLimit é o limite final desde created_at (VIAGEM_LIMITE_FINAL).
	FinalLimit time.Duration
	// Timeline são os parâmetros da incerteza da hora (os mesmos do recálculo de tempo).
	Timeline timeline.Config
}

// Calculator avalia e calcula as viagens.
type Calculator struct {
	repos port.Repositories // fora de transação
	// tx roda o cálculo (statement_timeout longo); failTx, a marcação de falha (outra transação, curta).
	tx, failTx port.Transactor
	met        port.MetricsWorker
	now        port.Clock
	log        *slog.Logger
	cfg        Config
}

// New liga o caso de uso às portas.
func New(repos port.Repositories, tx, failTx port.Transactor, met port.MetricsWorker, now port.Clock, log *slog.Logger, cfg Config) *Calculator {
	return &Calculator{repos: repos, tx: tx, failTx: failTx, met: met, now: now, log: log, cfg: cfg}
}

// Ready é uma viagem cujos dados já chegaram. ByLimit diz que o limite final foi o motivo (dados_parciais).
type Ready struct {
	ID      uuid.UUID
	ByLimit bool
}

// Evaluate avalia, fora de transação, as viagens em pending ou waiting_data: a primeira avaliação grava waiting_data, resolve
// boot_generation nula e devolve as que já podem ser calculadas.
func (c *Calculator) Evaluate(ctx context.Context) ([]Ready, error) {
	waiting, err := c.repos.Trips.ListWaiting(ctx)
	if err != nil {
		return nil, err
	}
	var ready []Ready
	for _, t := range waiting {
		if t.Status == trip.StatusPending {
			if err := c.repos.Trips.MarkWaiting(ctx, t.ID); err != nil {
				return ready, err
			}
		}
		if t.EntryType == "boot" && t.BootGeneration == nil && t.BootID != nil {
			if g, ok, err := c.repos.Trips.ResolveGeneration(ctx, t.ID, t.DongleID, *t.BootID); err != nil {
				return ready, err
			} else if ok {
				t.BootGeneration = &g
			}
		}
		ok, byLimit, err := c.isReady(ctx, t)
		if err != nil {
			return ready, err
		}
		if ok {
			ready = append(ready, Ready{ID: t.ID, ByLimit: byLimit})
		}
	}
	return ready, nil
}

// isReady aplica a regra de espera: limite final, ou contiguous_seq alcançou o alvo e há evidência de que o dongle parou
// (silêncio de lotes) ou, no intervalo, chegou um bloco com hora a partir de t1.
func (c *Calculator) isReady(ctx context.Context, t trip.Trip) (ready, byLimit bool, err error) {
	now := c.now()
	prog, err := c.repos.States.ReadProgress(ctx, t.DongleID)
	if err != nil {
		return false, false, err
	}
	complete := false
	if prog.ContiguousSeq >= t.TargetSeq {
		complete = prog.LastAckAt == nil || now.Sub(*prog.LastAckAt) >= c.cfg.WaitNoBatch
		if !complete && t.EntryType == "interval" && t.IntervalEnd != nil {
			ms, found, err := c.repos.TripSources.MaxBlockTime(ctx, t.DongleID, max(1, prog.ContiguousSeq-blockWindow+1), prog.ContiguousSeq)
			if err != nil {
				return false, false, err
			}
			complete = found && ms >= float64(t.IntervalEnd.UnixMilli())
		}
	}
	limit := now.Sub(t.CreatedAt) >= c.cfg.FinalLimit
	return complete || limit, limit && !complete, nil
}

// blockWindow é quantos blocos recentes MaxBlockTime examina (a evidência só precisa dos últimos blocos).
const blockWindow = 200

// Calculate calcula uma viagem pending ou waiting_data. found=false se outra transação a tem ou ela já mudou de estado.
func (c *Calculator) Calculate(ctx context.Context, id uuid.UUID, byLimit bool) (bool, error) {
	return c.process(ctx, func(ctx context.Context, r port.Repositories) (trip.Trip, bool, error) {
		return r.Trips.Take(ctx, id)
	}, byLimit)
}

// Recalc recalcula a viagem pronta com recalc_pending mais antiga. found=false se a fila está vazia.
func (c *Calculator) Recalc(ctx context.Context) (bool, error) {
	return c.process(ctx, func(ctx context.Context, r port.Repositories) (trip.Trip, bool, error) {
		return r.Trips.TakeRecalc(ctx)
	}, false)
}

// process trava a viagem, carrega, calcula e grava numa transação só. Erro (exceto port.ErrDatabaseBusy, que tenta de novo no
// próximo ciclo) ou pânico desfaz a transação e roda Fail noutra: uma viagem nova vira failed; um recálculo mantém o resultado.
func (c *Calculator) process(ctx context.Context, take func(context.Context, port.Repositories) (trip.Trip, bool, error), byLimit bool) (found bool, err error) {
	var taken trip.Trip
	var info calcInfo
	start := time.Now()
	err = c.tx.InTransaction(ctx, func(ctx context.Context, r port.Repositories) (err error) {
		defer func() {
			if p := recover(); p != nil {
				err = fmt.Errorf("pânico no cálculo da viagem: %v", p)
			}
		}()
		found = false
		t, ok, err := take(ctx, r)
		if err != nil || !ok {
			return err
		}
		found, taken = true, t
		calc, in, err := c.compute(ctx, r, t, byLimit)
		if err != nil {
			return err
		}
		info = in
		return r.Trips.Save(ctx, t.ID, calc)
	})
	if err != nil {
		if found && !errors.Is(err, port.ErrDatabaseBusy) && ctx.Err() == nil {
			c.fail(ctx, taken, err)
		}
		return found, err
	}
	if found {
		c.done(taken, info, time.Since(start))
	}
	return found, nil
}

func (c *Calculator) fail(ctx context.Context, t trip.Trip, cause error) {
	err := c.failTx.InTransaction(ctx, func(ctx context.Context, r port.Repositories) error { return r.Trips.Fail(ctx, t.ID) })
	if err != nil {
		c.log.Error("worker.viagem_falhou", "trip_id", t.ID, "erro", cause, "erro_marcacao", err)
		return
	}
	if t.Status == trip.StatusReady {
		c.log.Error("worker.viagem_recalculo_falhou", "trip_id", t.ID, "erro", cause)
		return
	}
	c.met.Trip("failed")
	c.log.Error("worker.viagem_falhou", "trip_id", t.ID, "dongle_id", t.DongleID, "erro", cause)
}

// calcInfo é o que o log e as métricas pedem do cálculo.
type calcInfo struct {
	parts, readings int
	provisional     bool
	partial         bool
	through         int64
}

func (c *Calculator) done(t trip.Trip, in calcInfo, d time.Duration) {
	result := "ready"
	if t.Status == trip.StatusReady {
		result = "recalculated"
	}
	c.met.Trip(result)
	if in.partial {
		c.met.TripPartial()
	}
	now := c.now()
	c.met.TripDuration(d)
	c.met.TripWait(now.Sub(t.CreatedAt))
	c.log.Info("worker.viagem_calculada", "trip_id", t.ID, "dongle_id", t.DongleID, "entrada", t.EntryType, "partes", in.parts,
		"leituras", in.readings, "provisoria", in.provisional, "dados_parciais", in.partial, "calculated_through_seq", in.through,
		"duracao_ms", d.Milliseconds())
}

// compute lê o avanço da ingestão (que vira calculated_through_seq), carrega os dados e calcula a análise.
func (c *Calculator) compute(ctx context.Context, r port.Repositories, t trip.Trip, byLimit bool) (trip.Calculation, calcInfo, error) {
	var info calcInfo
	prog, err := r.States.ReadProgress(ctx, t.DongleID)
	if err != nil {
		return trip.Calculation{}, info, err
	}
	params, err := tripanalysis.DefaultParams().WithFuel(t.Fuel)
	if err != nil {
		return trip.Calculation{}, info, err
	}
	l, err := c.load(ctx, r, t)
	if err != nil {
		return trip.Calculation{}, info, err
	}
	info.partial = byLimit || prog.ContiguousSeq < t.TargetSeq
	res, err := tripanalysis.Analyze(tripanalysis.Input{Entry: t.Entry(), Parts: l.parts, Params: params, EntryWarnings: l.warnings, Partial: info.partial})
	if err != nil {
		return trip.Calculation{}, info, err
	}
	proof, err := c.completeProof(ctx, r, t, l.boots, prog.ContiguousSeq)
	if err != nil {
		return trip.Calculation{}, info, err
	}
	// um boot sem hora pode ganhar conversão depois: a viagem fica provisória até lá, mesmo com prova de completude
	proof = proof && len(l.warnings) == 0
	info.parts, info.provisional, info.through = len(res.Parts), !proof, prog.ContiguousSeq
	info.readings = int(res.Summary.NReadings)
	return trip.Calculation{Result: res, FirstSeq: l.firstSeq, LastSeq: l.lastSeq, CalculatedThroughSeq: prog.ContiguousSeq,
		Provisional: !proof, CalculatedAt: c.now()}, info, nil
}

// completeProof diz se há prova de que a viagem está completa no contiguous_seq lido: um boot seguinte já começou (para o
// boot, ou para o último boot do intervalo) ou, no intervalo, chegou um bloco com hora a partir de t1.
func (c *Calculator) completeProof(ctx context.Context, r port.Repositories, t trip.Trip, boots []trip.Boot, contiguous int64) (bool, error) {
	var last *trip.Boot
	for i := range boots {
		if boots[i].LastSeq != nil && (last == nil || *boots[i].LastSeq > *last.LastSeq) {
			last = &boots[i]
		}
	}
	if last != nil {
		next, found, err := r.TripSources.NextBootStart(ctx, t.DongleID, *last.LastSeq)
		if err != nil {
			return false, err
		}
		if found && next <= contiguous {
			return true, nil
		}
	}
	if t.EntryType == "interval" && t.IntervalEnd != nil {
		ms, found, err := r.TripSources.MaxBlockTime(ctx, t.DongleID, max(1, contiguous-blockWindow+1), contiguous)
		if err != nil {
			return false, err
		}
		return found && ms >= float64(t.IntervalEnd.UnixMilli()), nil
	}
	return false, nil
}

// Queue é o tamanho das filas de viagens (para o gauge do worker).
func (c *Calculator) Queue(ctx context.Context) (trip.Queue, error) {
	return c.repos.Trips.CountQueue(ctx)
}
