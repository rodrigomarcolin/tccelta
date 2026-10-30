// Package boottime recalcula a reta de tempo de cada boot pendente e mantém as rotinas periódicas do worker
// (spec 06): o cálculo por boot, a marcação dos vizinhos sem âncora e a expiração de desafios. Depende só do domínio
// e das portas; o agendamento (LISTEN, intervalos, concorrência) fica em internal/worker.
package boottime

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"tccelta/backend/internal/domain/occurrence"
	"tccelta/backend/internal/domain/timeline"
	"tccelta/backend/internal/port"
)

// Params são os parâmetros do cálculo de tempo.
type Params struct{ Timeline timeline.Config }

// Processor processa a fila de boots pendentes.
type Processor struct {
	repos port.Repositories // fora de transação
	// tx roda o recálculo do boot; neighborTx, a marcação dos vizinhos (outra transação, com lock_timeout).
	tx, neighborTx port.Transactor
	met            port.MetricsWorker
	now            port.Clock
	log            *slog.Logger
	p              Params
}

// New liga o caso de uso às portas.
func New(repos port.Repositories, tx, neighborTx port.Transactor, met port.MetricsWorker, now port.Clock, log *slog.Logger, p Params) *Processor {
	return &Processor{repos: repos, tx: tx, neighborTx: neighborTx, met: met, now: now, log: log, p: p}
}

// ProcessOne pega um boot pendente e recalcula. Devolve false se a fila estava vazia.
func (c *Processor) ProcessOne(ctx context.Context) (bool, error) {
	t0 := time.Now()
	var processed, anchorsChanged bool
	var dongleID int32
	var seqFirst *int64
	err := c.tx.InTransaction(ctx, func(ctx context.Context, r port.Repositories) error {
		processed, anchorsChanged = false, false
		b, found, err := r.Boots.TakePending(ctx)
		if err != nil {
			return err
		}
		if !found {
			return nil
		}
		processed = true
		dongleID, seqFirst = b.DongleID, b.FirstSeq
		cfg := c.p.Timeline
		anchs, err := r.Boots.ListAnchors(ctx, b.BootKey)
		if err != nil {
			return err
		}
		var est timeline.Estimate
		switch {
		case len(anchs) > 0:
			est = timeline.EstimateFromAnchors(anchs, cfg)
		case b.FirstSeq != nil && b.FirstTMs != nil && b.LastTMs != nil:
			prev, err := r.Boots.PreviousAnchored(ctx, b.DongleID, *b.FirstSeq)
			if err != nil {
				return err
			}
			next, err := r.Boots.NextAnchored(ctx, b.DongleID, *b.FirstSeq)
			if err != nil {
				return err
			}
			est = timeline.EstimateFromNeighbors(prev, next, timeline.BootRange{FirstTMs: *b.FirstTMs, LastTMs: *b.LastTMs}, b.MaxOffsetMs, cfg)
		}
		now := c.now()
		var upd *timeline.Estimate
		if est.Source != "" {
			upd = &est
		}
		if err := r.Boots.UpdateTime(ctx, b.BootKey, upd, now); err != nil {
			return err
		}
		if err := r.AnchorGroups.Delete(ctx, b.BootKey); err != nil {
			return err
		}
		if est.Source == "anchors" {
			if err := r.AnchorGroups.Copy(ctx, b.BootKey, now, est.Groups); err != nil {
				return err
			}
			c.met.Groups(len(est.Groups))
			anchorsChanged = b.AMs == nil || b.TimeSource == nil || *b.TimeSource != "anchors" || *b.AMs != est.AMs || *b.B != est.B
		}
		for _, a := range est.Alerts {
			det, _ := json.Marshal(a.Detail)
			gen, boot := b.Generation, b.BootID
			if err := r.Occurrences.InsertAlertIfNew(ctx, occurrence.Alert{DongleID: b.DongleID, BootGeneration: &gen, BootID: &boot,
				Type: a.Type, Detail: det}, now); err != nil {
				return err
			}
			c.met.Alert(a.Type)
		}
		source := est.Source
		if source == "" {
			source = "no_estimate"
		}
		c.met.Boot(source)
		c.log.Info("worker.boot_recalculado", "dongle_id", b.DongleID, "geracao_boot", b.Generation, "boot_id", b.BootID,
			"origem", source, "a_ms", est.AMs, "b_ppm", est.B*1e6, "incerteza_ms", est.BaseUncertaintyMs, "grupos", len(est.Groups), "anchors", len(anchs))
		return nil
	})
	if err != nil {
		return false, err
	}
	if !processed {
		return false, nil
	}
	c.met.BootDuration(time.Since(t0))
	if anchorsChanged && seqFirst != nil {
		// 2ª transação: vizinhos sem âncora, sob o advisory lock do dongle
		err := c.neighborTx.InTransaction(ctx, func(ctx context.Context, r port.Repositories) error {
			if err := r.Dongles.Lock(ctx, dongleID); err != nil {
				return err
			}
			_, err := r.Boots.MarkAnchorlessNeighbors(ctx, dongleID, *seqFirst, c.now())
			return err
		})
		if err != nil {
			c.log.Warn("worker.vizinhos_adiados", "dongle_id", dongleID, "erro", err)
		}
	}
	return true, nil
}

// ExpireChallenges marca como expirados os desafios vencidos.
func (c *Processor) ExpireChallenges(ctx context.Context) (int64, error) {
	n, err := c.repos.Challenges.Expire(ctx, c.now())
	if err == nil {
		c.met.Expired(n)
	}
	return n, err
}

// PendingCount é o tamanho da fila de boots pendentes.
func (c *Processor) PendingCount(ctx context.Context) (int64, error) {
	return c.repos.Boots.CountPending(ctx)
}
