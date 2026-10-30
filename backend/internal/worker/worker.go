// Package worker é o runtime assíncrono do recálculo de tempo (spec 06): escuta boot_pending, agenda o dreno da fila,
// roda a expiração de desafios, atualiza o gauge da fila e serve a saúde. A lógica de negócio está em usecase/boottime.
package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"tccelta/backend/internal/config"
	"tccelta/backend/internal/port"
	"tccelta/backend/internal/usecase/boottime"
)

// Worker processa a fila de boots pendentes.
type Worker struct {
	Boots   *boottime.Processor
	Signals port.Notifications
	Cfg     config.WorkerConfig
	Log     *slog.Logger
	Met     port.MetricsWorker
}

// Drain processa boots até a fila esvaziar.
func (w *Worker) Drain(ctx context.Context) {
	for ctx.Err() == nil {
		ok, err := w.Boots.ProcessOne(ctx)
		if err != nil {
			w.Log.Error("worker.erro", "erro", err)
			return
		}
		if !ok {
			return
		}
	}
}

// Run roda o worker até o contexto terminar.
func (w *Worker) Run(ctx context.Context, healthAddr string) error {
	signals := w.Signals.Listen(ctx, "boot_pending", w.Met.Reconnection)
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
				if _, err := w.Boots.ExpireChallenges(ctx); err != nil {
					w.Log.Error("worker.expiracao_falhou", "erro", err)
				}
			case <-f.C:
				if c, err := w.Boots.PendingCount(ctx); err == nil {
					w.Met.Queue(c)
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
