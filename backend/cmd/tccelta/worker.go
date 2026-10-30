package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"net/http"

	"tccelta/backend/internal/adapter/postgres"
	metrics "tccelta/backend/internal/adapter/prometheus"
	"tccelta/backend/internal/config"
	"tccelta/backend/internal/database"
	"tccelta/backend/internal/database/dbgen"
	"tccelta/backend/internal/domain/pid"
	"tccelta/backend/internal/domain/protocol"
	"tccelta/backend/internal/domain/timeline"
	"tccelta/backend/internal/port"
	"tccelta/backend/internal/port/porttest"
	"tccelta/backend/internal/usecase/boottime"
	"tccelta/backend/internal/usecase/catalog"
	"tccelta/backend/internal/usecase/reprocess"
	"tccelta/backend/internal/worker"
)

func cmdWorker(ctx context.Context, args []string) error {
	if len(args) > 0 && args[0] == "reprocess" {
		return cmdReprocess(ctx, args[1:])
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := newLogger(cfg)
	met := metrics.New()
	pool, err := database.NewPool(ctx, cfg.Database, log)
	if err != nil {
		return err
	}
	defer pool.Close()
	met.RegisterPool(poolStats(pool))
	go func() {
		addr := os.Getenv("METRICAS_ENDERECO")
		if addr == "" {
			addr = ":9091"
		}
		mux := http.NewServeMux()
		mux.Handle("GET /metricas", met.Handler())
		_ = (&http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}).ListenAndServe()
	}()
	// o recálculo do boot roda só com statement_timeout; a marcação dos vizinhos (2ª transação) usa o padrão, com lock_timeout
	repos := postgres.Repositories(dbgen.New(pool))
	tcfg := timeline.Config{GroupWindowMs: float64(cfg.Worker.GroupWindow.Milliseconds()), MaxDriftPPM: cfg.Worker.MaxDriftPPM,
		MinHalfWidthMs: cfg.Worker.MinHalfWidthMs, SpanMinMs: float64(cfg.Worker.SpanMin.Milliseconds())}
	boots := boottime.New(repos, postgres.NewTransactorWith(pool, database.TxOptions{StatementTimeout: 10 * time.Second}),
		postgres.NewTransactor(pool), met.WorkerPort(), port.RealClock, log, boottime.Params{Timeline: tcfg})
	w := &worker.Worker{Boots: boots, Signals: postgres.NewNotifications(pool, log), Cfg: cfg.Worker, Log: log, Met: met.WorkerPort()}
	log.Info("worker.iniciado")
	return w.Run(ctx, cfg.HTTP.WorkerHealth)
}

func cmdReprocess(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("reprocess", flag.ContinueOnError)
	dongle := fs.String("dongle", "", "DevId (16 hex)")
	since := fs.Int64("from-seq", 1, "seq inicial")
	until := fs.Int64("to-seq", 0, "seq final (0 = até o fim)")
	batch := fs.Int("batch", 200, "blocos por janela")
	apply := fs.Bool("apply", false, "aplica (sem isto é só um ensaio)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	dev, err := protocol.DevIDFromHex(*dongle)
	if err != nil {
		return fmt.Errorf("--dongle: %w", err)
	}
	cfg, _ := config.Load()
	log := newLogger(cfg)
	pool, err := poolFromEnv(ctx, "ADMIN_", log)
	if err != nil {
		return err
	}
	defer pool.Close()
	repos := postgres.Repositories(dbgen.New(pool))
	d, err := repos.Dongles.FindByDevID(ctx, dev)
	if err != nil {
		return fmt.Errorf("dongle não cadastrado: %w", err)
	}
	cat := pid.New()
	if err := catalog.NewSync(repos.Pids, cat, log).Reload(ctx); err != nil {
		return err
	}
	uc := reprocess.New(repos, postgres.NewTransactor(pool), cat, porttest.Nop{}, log)
	r, err := uc.Run(ctx, reprocess.Options{DongleID: d.ID, FromSeq: *since, ToSeq: *until,
		Batch: int32(*batch), Apply: *apply, Operator: os.Getenv("USER")})
	mode := "ENSAIO (nada foi escrito; use --apply)"
	if *apply {
		mode = "APLICADO"
	}
	fmt.Printf("%s: blocos=%d linhas_atuais=%d linhas_novas=%d último_seq=%s\n", mode, r.Blocks, r.RemovedRows, r.InsertedRows, strconv.FormatInt(r.LastSeq, 10))
	return err
}

func cmdMigrate(ctx context.Context, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	dsn, err := database.DSN(cfg.Database)
	if err != nil {
		return err
	}
	pc, err := pgx.ParseConfig(dsn)
	if err != nil {
		return err
	}
	db := stdlib.OpenDB(*pc)
	defer db.Close()
	action := "up"
	if len(args) > 0 {
		action = args[0]
	}
	switch action {
	case "up":
		if err := database.Migrate(ctx, db); err != nil {
			return err
		}
		fmt.Println("migrations aplicadas")
	case "down-to":
		if len(args) < 2 {
			return fmt.Errorf("uso: migrate down-to N")
		}
		n, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil {
			return err
		}
		return database.DownTo(ctx, db, n)
	case "status":
		st, err := database.Status(ctx, db)
		if err != nil {
			return err
		}
		for _, s := range st {
			fmt.Printf("%05d %s %s\n", s.Source.Version, s.State, s.Source.Path)
		}
	default:
		return fmt.Errorf("ação desconhecida %q", action)
	}
	return nil
}
