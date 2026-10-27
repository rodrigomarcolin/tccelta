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

	"tccelta/backend/internal/banco"
	"tccelta/backend/internal/banco/consultas"
	"tccelta/backend/internal/config"
	"tccelta/backend/internal/metricas"
	"tccelta/backend/internal/tempo"
)

// Worker processa a fila de boots pendentes.
type Worker struct {
	Pool  *pgxpool.Pool
	Cfg   config.WorkerConfig
	Log   *slog.Logger
	Met   *metricas.M
	Agora func() time.Time
}

func (w *Worker) agora() time.Time {
	if w.Agora != nil {
		return w.Agora()
	}
	return time.Now()
}

func (w *Worker) tcfg() tempo.Config {
	return tempo.Config{JanelaGrupoMs: float64(w.Cfg.JanelaGrupo.Milliseconds()), DerivaMaxPPM: w.Cfg.DerivaMaxPPM,
		MeiaLarguraMinMs: w.Cfg.MeiaLarguraMinMs, SpanMinMs: float64(w.Cfg.SpanMin.Milliseconds())}
}

func ms(t time.Time) float64 { return float64(t.UnixMicro()) / 1000 }

// ProcessarUmBoot pega um boot pendente e recalcula. Devolve false se a fila estava vazia.
func (w *Worker) ProcessarUmBoot(ctx context.Context) (bool, error) {
	t0 := time.Now()
	var processou, mudouAncoras bool
	var dongleID int32
	var seqPrimeiro *int64
	err := banco.EmTransacao(ctx, w.Pool, banco.OpcoesTx{StatementTimeout: 10 * time.Second},
		func(ctx context.Context, q *consultas.Queries, tx pgx.Tx) error {
			processou, mudouAncoras = false, false
			b, err := q.PegarBootPendente(ctx)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return nil
				}
				return err
			}
			processou = true
			dongleID, seqPrimeiro = b.DongleID, b.SeqPrimeiro
			cfg := w.tcfg()
			ancs, err := q.ListarAncorasDoBoot(ctx, b.DongleID, b.GeracaoBoot, b.BootID)
			if err != nil {
				return err
			}
			var est tempo.Estimativa
			switch {
			case len(ancs) > 0:
				as := make([]tempo.Ancora, len(ancs))
				for i, a := range ancs {
					as[i] = tempo.Ancora{TMs: a.TMs, S1Ms: ms(a.S1), S2Ms: ms(a.S2)}
				}
				est = tempo.EstimarPorAncoras(as, cfg)
			case b.SeqPrimeiro != nil && b.TMsPrimeiro != nil && b.TMsUltimo != nil:
				var ant, prox *tempo.ReferenciaVizinho
				if r, err := q.BootAncoradoAnterior(ctx, b.DongleID, b.SeqPrimeiro); err == nil {
					ant = refDe(r.AMs, r.B, r.IncertezaBaseMs, r.TMsPrimeiro, r.TMsUltimo)
				} else if !errors.Is(err, pgx.ErrNoRows) {
					return err
				}
				if r, err := q.BootAncoradoSeguinte(ctx, b.DongleID, b.SeqPrimeiro); err == nil {
					prox = refDe(r.AMs, r.B, r.IncertezaBaseMs, r.TMsPrimeiro, r.TMsUltimo)
				} else if !errors.Is(err, pgx.ErrNoRows) {
					return err
				}
				est = tempo.EstimarPorVizinhos(ant, prox, tempo.FaixaBoot{TMsPrimeiro: *b.TMsPrimeiro, TMsUltimo: *b.TMsUltimo}, b.DeslocamentoMaxMs, cfg)
			}
			agora := w.agora()
			if est.Origem != "" {
				ia, ib := est.AMs, est.B
				inc := int32(est.IncertezaBaseMs)
				o := est.Origem
				err = q.AtualizarTempoBoot(ctx, consultas.AtualizarTempoBootParams{AMs: &ia, B: &ib, IncertezaBaseMs: &inc,
					OrigemTempo: &o, Agora: &agora, DongleID: b.DongleID, GeracaoBoot: b.GeracaoBoot, BootID: b.BootID})
			} else {
				err = q.AtualizarTempoBoot(ctx, consultas.AtualizarTempoBootParams{Agora: &agora, DongleID: b.DongleID, GeracaoBoot: b.GeracaoBoot, BootID: b.BootID})
			}
			if err != nil {
				return err
			}
			if err := q.ApagarGruposAncora(ctx, b.DongleID, b.GeracaoBoot, b.BootID); err != nil {
				return err
			}
			if est.Origem == "ancoras" {
				linhas := make([]consultas.CopiarGruposAncoraParams, len(est.Grupos))
				for i, g := range est.Grupos {
					linhas[i] = consultas.CopiarGruposAncoraParams{DongleID: b.DongleID, GeracaoBoot: b.GeracaoBoot, BootID: b.BootID,
						Ordem: int32(i + 1), TPrimeiroMs: g.TPrimeiro, TUltimoMs: g.TUltimo, TCentroMs: g.TCentro, LoMs: g.Lo, HiMs: g.Hi,
						NAncoras: int32(g.N), CalculadoEm: agora}
				}
				if _, err := q.CopiarGruposAncora(ctx, linhas); err != nil {
					return err
				}
				if w.Met != nil {
					w.Met.WorkerGrupos.Add(float64(len(linhas)))
				}
				mudouAncoras = b.AMs == nil || b.OrigemTempo == nil || *b.OrigemTempo != "ancoras" || *b.AMs != est.AMs || *b.B != est.B
			}
			for _, a := range est.Alertas {
				det, _ := json.Marshal(a.Detalhe)
				if err := q.InserirAlertaSeNovo(ctx, consultas.InserirAlertaSeNovoParams{DongleID: b.DongleID, GeracaoBoot: b.GeracaoBoot,
					BootID: b.BootID, Tipo: a.Tipo, Detalhe: det, Agora: agora}); err != nil {
					return err
				}
				if w.Met != nil {
					w.Met.WorkerAlertas.WithLabelValues(a.Tipo).Inc()
				}
			}
			origem := est.Origem
			if origem == "" {
				origem = "sem_estimativa"
			}
			if w.Met != nil {
				w.Met.WorkerBoots.WithLabelValues(origem).Inc()
			}
			w.Log.Info("worker.boot_recalculado", "dongle_id", b.DongleID, "geracao_boot", b.GeracaoBoot, "boot_id", b.BootID,
				"origem", origem, "a_ms", est.AMs, "b_ppm", est.B*1e6, "incerteza_ms", est.IncertezaBaseMs, "grupos", len(est.Grupos), "ancoras", len(ancs))
			return nil
		})
	if err != nil {
		return false, err
	}
	if !processou {
		return false, nil
	}
	if w.Met != nil {
		w.Met.WorkerDur.Observe(time.Since(t0).Seconds())
	}
	if mudouAncoras && seqPrimeiro != nil {
		// TX2: vizinhos sem âncora, sob o advisory lock do dongle
		err := banco.EmTransacao(ctx, w.Pool, banco.OpcoesTx{LockTimeout: 3 * time.Second, StatementTimeout: 10 * time.Second},
			func(ctx context.Context, q *consultas.Queries, tx pgx.Tx) error {
				if err := q.TravarDongle(ctx, dongleID); err != nil {
					return err
				}
				_, err := q.MarcarVizinhosSemAncora(ctx, w.agora(), dongleID, seqPrimeiro)
				return err
			})
		if err != nil {
			w.Log.Warn("worker.vizinhos_adiados", "dongle_id", dongleID, "erro", err)
		}
	}
	return true, nil
}

func refDe(a, b *float64, inc *int32, tp, tu *int64) *tempo.ReferenciaVizinho {
	if a == nil || b == nil || inc == nil || tp == nil || tu == nil {
		return nil
	}
	return &tempo.ReferenciaVizinho{AMs: *a, B: *b, IncertezaBaseMs: int(*inc), TMsPrimeiro: *tp, TMsUltimo: *tu}
}

// Drenar processa boots até a fila esvaziar.
func (w *Worker) Drenar(ctx context.Context) {
	for ctx.Err() == nil {
		ok, err := w.ProcessarUmBoot(ctx)
		if err != nil {
			w.Log.Error("worker.erro", "erro", err)
			return
		}
		if !ok {
			return
		}
	}
}

// ExpirarDesafios marca como expirados os desafios vencidos.
func (w *Worker) ExpirarDesafios(ctx context.Context) (int64, error) {
	n, err := consultas.New(w.Pool).ExpirarDesafios(ctx, w.agora())
	if err == nil && w.Met != nil {
		w.Met.WorkerExpirados.Add(float64(n))
	}
	return n, err
}

// Executar roda o worker até o contexto terminar.
func (w *Worker) Executar(ctx context.Context, saudeAddr string) error {
	var reconex func()
	if w.Met != nil {
		reconex = func() { w.Met.WorkerReconex.Inc() }
	}
	sinais := banco.Escutar(ctx, w.Pool, "boot_pendente", w.Log, reconex)
	acordar := make(chan struct{}, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-sinais:
				if !ok {
					return
				}
				select {
				case acordar <- struct{}{}:
				default:
				}
			}
		}
	}()
	n := max(1, w.Cfg.Concorrencia)
	canais := make([]chan struct{}, n)
	for i := range canais {
		canais[i] = make(chan struct{}, 1)
		wg.Add(1)
		go func(c chan struct{}) {
			defer wg.Done()
			t := time.NewTicker(w.Cfg.IntervaloPoll)
			defer t.Stop()
			for {
				w.Drenar(ctx)
				select {
				case <-ctx.Done():
					return
				case <-c:
				case <-t.C:
				}
			}
		}(canais[i])
	}
	wg.Add(1)
	go func() { // distribui o sinal coalescido
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case <-acordar:
				for _, c := range canais {
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
		t := time.NewTicker(w.Cfg.IntervaloExpiracao)
		f := time.NewTicker(15 * time.Second)
		defer t.Stop()
		defer f.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if _, err := w.ExpirarDesafios(ctx); err != nil {
					w.Log.Error("worker.expiracao_falhou", "erro", err)
				}
			case <-f.C:
				if c, err := consultas.New(w.Pool).ContarBootsPendentes(ctx); err == nil && w.Met != nil {
					w.Met.WorkerFila.Set(float64(c))
				}
			}
		}
	}()
	var srv *http.Server
	if saudeAddr != "" {
		mux := http.NewServeMux()
		mux.HandleFunc("GET /saude/viva", func(rw http.ResponseWriter, r *http.Request) { rw.WriteHeader(http.StatusOK) })
		srv = &http.Server{Addr: saudeAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
		go func() {
			if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				w.Log.Error("worker.saude", "erro", fmt.Sprint(err))
			}
		}()
	}
	<-ctx.Done()
	if srv != nil {
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = srv.Shutdown(sctx)
		cancel()
	}
	wg.Wait()
	return nil
}
