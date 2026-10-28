// Package worker recalcula a reta de tempo de cada boot (spec 06).
package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"tccelta/backend/internal/config"
	"tccelta/backend/internal/database"
	"tccelta/backend/internal/database/dbgen"
	"tccelta/backend/internal/domain/timeline"
	"tccelta/backend/internal/observability/metrics"
)

// Worker processa a fila de boots pendentes.
type Worker struct {
	Pool *pgxpool.Pool
	Cfg  config.WorkerConfig
	Log  *slog.Logger
	Met  *metrics.M
	Now  func() time.Time
}

func (w *Worker) now() time.Time {
	if w.Now != nil {
		return w.Now()
	}
	return time.Now()
}

func (w *Worker) tcfg() timeline.Config {
	return timeline.Config{GroupWindowMs: float64(w.Cfg.GroupWindow.Milliseconds()), MaxDriftPPM: w.Cfg.MaxDriftPPM,
		MinHalfWidthMs: w.Cfg.MinHalfWidthMs, SpanMinMs: float64(w.Cfg.SpanMin.Milliseconds())}
}

func ms(t time.Time) float64 { return float64(t.UnixMicro()) / 1000 }

// ProcessOneBoot pega um boot pendente e recalcula. Devolve false se a fila estava vazia.
func (w *Worker) ProcessOneBoot(ctx context.Context) (bool, error) {
	t0 := time.Now()
	var processed, anchorsChanged bool
	var dongleID int32
	var seqFirst *int64
	err := database.InTransaction(ctx, w.Pool, database.TxOptions{StatementTimeout: 10 * time.Second},
		func(ctx context.Context, q *dbgen.Queries, tx pgx.Tx) error {
			processed, anchorsChanged = false, false
			b, err := q.TakePendingBoot(ctx)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return nil
				}
				return err
			}
			processed = true
			dongleID, seqFirst = b.DongleID, b.FirstSeq
			cfg := w.tcfg()
			anchs, err := q.ListBootAnchors(ctx, b.DongleID, b.BootGeneration, b.BootID)
			if err != nil {
				return err
			}
			var est timeline.Estimate
			switch {
			case len(anchs) > 0:
				as := make([]timeline.Anchor, len(anchs))
				for i, a := range anchs {
					as[i] = timeline.Anchor{TMs: a.TMs, S1Ms: ms(a.S1), S2Ms: ms(a.S2)}
				}
				est = timeline.EstimateFromAnchors(as, cfg)
			case b.FirstSeq != nil && b.FirstTMs != nil && b.LastTMs != nil:
				var prev, next *timeline.NeighborReference
				if r, err := q.PreviousAnchoredBoot(ctx, b.DongleID, b.FirstSeq); err == nil {
					prev = refFrom(r.AMs, r.B, r.BaseUncertaintyMs, r.FirstTMs, r.LastTMs)
				} else if !errors.Is(err, pgx.ErrNoRows) {
					return err
				}
				if r, err := q.NextAnchoredBoot(ctx, b.DongleID, b.FirstSeq); err == nil {
					next = refFrom(r.AMs, r.B, r.BaseUncertaintyMs, r.FirstTMs, r.LastTMs)
				} else if !errors.Is(err, pgx.ErrNoRows) {
					return err
				}
				est = timeline.EstimateFromNeighbors(prev, next, timeline.BootRange{FirstTMs: *b.FirstTMs, LastTMs: *b.LastTMs}, b.MaxOffsetMs, cfg)
			}
			now := w.now()
			if est.Source != "" {
				ia, ib := est.AMs, est.B
				inc := int32(est.BaseUncertaintyMs)
				o := est.Source
				err = q.UpdateBootTime(ctx, dbgen.UpdateBootTimeParams{AMs: &ia, B: &ib, BaseUncertaintyMs: &inc,
					TimeSource: &o, Now: &now, DongleID: b.DongleID, BootGeneration: b.BootGeneration, BootID: b.BootID})
			} else {
				err = q.UpdateBootTime(ctx, dbgen.UpdateBootTimeParams{Now: &now, DongleID: b.DongleID, BootGeneration: b.BootGeneration, BootID: b.BootID})
			}
			if err != nil {
				return err
			}
			if err := q.DeleteAnchorGroups(ctx, b.DongleID, b.BootGeneration, b.BootID); err != nil {
				return err
			}
			if est.Source == "anchors" {
				rows := make([]dbgen.CopyAnchorGroupsParams, len(est.Groups))
				for i, g := range est.Groups {
					rows[i] = dbgen.CopyAnchorGroupsParams{DongleID: b.DongleID, BootGeneration: b.BootGeneration, BootID: b.BootID,
						Ordinal: int32(i + 1), FirstTMs: g.FirstT, LastTMs: g.LastT, TCenterMs: g.TCenter, LoMs: g.Lo, HiMs: g.Hi,
						NAnchors: int32(g.N), ComputedAt: now}
				}
				if _, err := q.CopyAnchorGroups(ctx, rows); err != nil {
					return err
				}
				if w.Met != nil {
					w.Met.WorkerGroups.Add(float64(len(rows)))
				}
				anchorsChanged = b.AMs == nil || b.TimeSource == nil || *b.TimeSource != "anchors" || *b.AMs != est.AMs || *b.B != est.B
			}
			for _, a := range est.Alerts {
				det, _ := json.Marshal(a.Detail)
				if err := q.InsertAlertIfNew(ctx, dbgen.InsertAlertIfNewParams{DongleID: b.DongleID, BootGeneration: b.BootGeneration,
					BootID: b.BootID, Type: a.Type, Detail: det, Now: now}); err != nil {
					return err
				}
				if w.Met != nil {
					w.Met.WorkerAlerts.WithLabelValues(a.Type).Inc()
				}
			}
			source := est.Source
			if source == "" {
				source = "no_estimate"
			}
			if w.Met != nil {
				w.Met.WorkerBoots.WithLabelValues(source).Inc()
			}
			w.Log.Info("worker.boot_recalculado", "dongle_id", b.DongleID, "geracao_boot", b.BootGeneration, "boot_id", b.BootID,
				"origem", source, "a_ms", est.AMs, "b_ppm", est.B*1e6, "incerteza_ms", est.BaseUncertaintyMs, "grupos", len(est.Groups), "anchors", len(anchs))
			return nil
		})
	if err != nil {
		return false, err
	}
	if !processed {
		return false, nil
	}
	if w.Met != nil {
		w.Met.WorkerDur.Observe(time.Since(t0).Seconds())
	}
	if anchorsChanged && seqFirst != nil {
		// TX2: vizinhos sem âncora, sob o advisory lock do dongle
		err := database.InTransaction(ctx, w.Pool, database.TxOptions{LockTimeout: 3 * time.Second, StatementTimeout: 10 * time.Second},
			func(ctx context.Context, q *dbgen.Queries, tx pgx.Tx) error {
				if err := q.LockDongle(ctx, dongleID); err != nil {
					return err
				}
				_, err := q.MarkAnchorlessNeighbors(ctx, w.now(), dongleID, seqFirst)
				return err
			})
		if err != nil {
			w.Log.Warn("worker.vizinhos_adiados", "dongle_id", dongleID, "erro", err)
		}
	}
	return true, nil
}

func refFrom(a, b *float64, inc *int32, tp, tu *int64) *timeline.NeighborReference {
	if a == nil || b == nil || inc == nil || tp == nil || tu == nil {
		return nil
	}
	return &timeline.NeighborReference{AMs: *a, B: *b, BaseUncertaintyMs: int(*inc), FirstTMs: *tp, LastTMs: *tu}
}

// Drain processa boots até a fila esvaziar.
func (w *Worker) Drain(ctx context.Context) {
	for ctx.Err() == nil {
		ok, err := w.ProcessOneBoot(ctx)
		if err != nil {
			w.Log.Error("worker.erro", "erro", err)
			return
		}
		if !ok {
			return
		}
	}
}

// ExpireChallenges marca como expirados os desafios vencidos.
func (w *Worker) ExpireChallenges(ctx context.Context) (int64, error) {
	n, err := dbgen.New(w.Pool).ExpireChallenges(ctx, w.now())
	if err == nil && w.Met != nil {
		w.Met.WorkerExpired.Add(float64(n))
	}
	return n, err
}

// Run roda o worker até o contexto terminar.
func (w *Worker) Run(ctx context.Context, healthAddr string) error {
	var reconnect func()
	if w.Met != nil {
		reconnect = func() { w.Met.WorkerReconnect.Inc() }
	}
	signals := database.Listen(ctx, w.Pool, "boot_pending", w.Log, reconnect)
	wake := make(chan struct{}, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-signals:
				if !ok {
					return
				}
				select {
				case wake <- struct{}{}:
				default:
				}
			}
		}
	}()
	n := max(1, w.Cfg.Concurrency)
	channels := make([]chan struct{}, n)
	for i := range channels {
		channels[i] = make(chan struct{}, 1)
		wg.Add(1)
		go func(c chan struct{}) {
			defer wg.Done()
			t := time.NewTicker(w.Cfg.PollInterval)
			defer t.Stop()
			for {
				w.Drain(ctx)
				select {
				case <-ctx.Done():
					return
				case <-c:
				case <-t.C:
				}
			}
		}(channels[i])
	}
	wg.Add(1)
	go func() { // distribui o sinal coalescido
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case <-wake:
				for _, c := range channels {
					select {
					case c <- struct{}{}:
					default:
					}
				}
			}
		}
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		t := time.NewTicker(w.Cfg.ExpirationInterval)
		f := time.NewTicker(15 * time.Second)
		defer t.Stop()
		defer f.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if _, err := w.ExpireChallenges(ctx); err != nil {
					w.Log.Error("worker.expiracao_falhou", "erro", err)
				}
			case <-f.C:
				if c, err := dbgen.New(w.Pool).CountPendingBoots(ctx); err == nil && w.Met != nil {
					w.Met.WorkerQueue.Set(float64(c))
				}
			}
		}
	}()
	var srv *http.Server
	if healthAddr != "" {
		mux := http.NewServeMux()
		mux.HandleFunc("GET /saude/viva", func(rw http.ResponseWriter, r *http.Request) { rw.WriteHeader(http.StatusOK) })
		srv = &http.Server{Addr: healthAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
		go func() {
			if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				w.Log.Error("worker.saude", "erro", fmt.Sprint(err))
			}
		}()
	}
	<-ctx.Done()
	if srv != nil {
		sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		_ = srv.Shutdown(sctx)
		cancel()
	}
	wg.Wait()
	return nil
}
