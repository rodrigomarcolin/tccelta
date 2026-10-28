package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"net/http"

	"tccelta/backend/internal/banco"
	"tccelta/backend/internal/banco/consultas"
	"tccelta/backend/internal/casodeuso/catalogo"
	"tccelta/backend/internal/config"
	"tccelta/backend/internal/dominio/pid"
	"tccelta/backend/internal/dominio/protocolo"
	"tccelta/backend/internal/observabilidade/metricas"
	"tccelta/backend/internal/repositorio/postgres"
	"tccelta/backend/internal/reprocessamento"
	"tccelta/backend/internal/worker"
)

func cmdWorker(ctx context.Context, args []string) error {
	if len(args) > 0 && args[0] == "reprocessar" {
		return cmdReprocessar(ctx, args[1:])
	}
	cfg, err := config.Carregar()
	if err != nil {
		return err
	}
	log := novoLog(cfg)
	met := metricas.Nova()
	pool, err := banco.NovoPool(ctx, cfg.Banco, log)
	if err != nil {
		return err
	}
	defer pool.Close()
	met.RegistrarPool(pool)
	go func() {
		addr := os.Getenv("METRICAS_ENDERECO")
		if addr == "" {
			addr = ":9091"
		}
		mux := http.NewServeMux()
		mux.Handle("GET /metricas", met.Handler())
		_ = (&http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}).ListenAndServe()
	}()
	w := &worker.Worker{Pool: pool, Cfg: cfg.Worker, Log: log, Met: met}
	log.Info("worker.iniciado")
	return w.Executar(ctx, cfg.HTTP.SaudeWorker)
}

func cmdReprocessar(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("reprocessar", flag.ContinueOnError)
	dongle := fs.String("dongle", "", "DevId (16 hex)")
	desde := fs.Int64("desde-seq", 1, "seq inicial")
	ate := fs.Int64("ate-seq", 0, "seq final (0 = até o fim)")
	lote := fs.Int("lote", 200, "blocos por janela")
	aplicar := fs.Bool("aplicar", false, "aplica (sem isto é só um ensaio)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	dev, err := protocolo.DevIDDeHex(*dongle)
	if err != nil {
		return fmt.Errorf("--dongle: %w", err)
	}
	cfg, _ := config.Carregar()
	log := novoLog(cfg)
	pool, err := poolDeEnv(ctx, "ADMIN_", log)
	if err != nil {
		return err
	}
	defer pool.Close()
	q := consultas.New(pool)
	d, err := q.BuscarDonglePorDevId(ctx, dev[:])
	if err != nil {
		return fmt.Errorf("dongle não cadastrado: %w", err)
	}
	cat := pid.Novo()
	if err := catalogo.NovoSincronizar(postgres.Repositorios(q).Pids, cat, log).Recarregar(ctx); err != nil {
		return err
	}
	r, err := reprocessamento.Reprocessar(ctx, pool, cat, reprocessamento.OpcoesReprocesso{DongleID: d.DongleID, DesdeSeq: *desde, AteSeq: *ate,
		Lote: int32(*lote), Aplicar: *aplicar, Operador: os.Getenv("USER")}, log, nil)
	modo := "ENSAIO (nada foi escrito; use --aplicar)"
	if *aplicar {
		modo = "APLICADO"
	}
	fmt.Printf("%s: blocos=%d linhas_atuais=%d linhas_novas=%d último_seq=%s\n", modo, r.Blocos, r.LinhasRemovidas, r.LinhasInseridas, strconv.FormatInt(r.UltimoSeq, 10))
	return err
}

func cmdMigrar(ctx context.Context, args []string) error {
	cfg, err := config.Carregar()
	if err != nil {
		return err
	}
	dsn, err := banco.DSN(cfg.Banco)
	if err != nil {
		return err
	}
	pc, err := pgx.ParseConfig(dsn)
	if err != nil {
		return err
	}
	var db *sql.DB = stdlib.OpenDB(*pc)
	defer db.Close()
	acao := "up"
	if len(args) > 0 {
		acao = args[0]
	}
	switch acao {
	case "up":
		if err := banco.Migrar(ctx, db); err != nil {
			return err
		}
		fmt.Println("migrations aplicadas")
	case "down-to":
		if len(args) < 2 {
			return fmt.Errorf("uso: migrar down-to N")
		}
		n, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil {
			return err
		}
		return banco.DescerAte(ctx, db, n)
	case "status":
		st, err := banco.Status(ctx, db)
		if err != nil {
			return err
		}
		for _, s := range st {
			fmt.Printf("%05d %s %s\n", s.Source.Version, s.State, s.Source.Path)
		}
	default:
		return fmt.Errorf("ação desconhecida %q", acao)
	}
	return nil
}
