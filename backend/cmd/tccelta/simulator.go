package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"tccelta/backend/internal/adapter/postgres"
	"tccelta/backend/internal/config"
	"tccelta/backend/internal/database/dbgen"
	"tccelta/backend/internal/tools/simulator"
)

func cmdSimulator(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("simulator", flag.ContinueOnError)
	scenario := fs.String("scenario", "e2e", "e2e|negative-protocol|time|generation|load|reconnection|trip|trip-http|client-auth|client-auth-rotation|knee")
	api := fs.String("api", "http://api:8080", "URL base da API")
	keys := fs.String("keys", "", "CSV de dongles de teste")
	dongles := fs.Int("dongles", 0, "número de dongles virtuais")
	duration := fs.Duration("duration", 2*time.Minute, "duração da carga")
	speedup := fs.Float64("speedup", 1, "fator de aceleração do T_lat")
	tlat := fs.Duration("tlat", 60*time.Second, "T_lat")
	accumulated := fs.Int("accumulated", 500, "blocos acumulados offline")
	ramp := fs.Duration("ramp", 30*time.Second, "rampa de conexão")
	dbEnv := fs.String("db-url-env", "SIM_BANCO_URL", "variável com a URL do banco (SELECT)")
	rel := fs.String("report", "", "arquivo do relatório JSON")
	drift := fs.Float64("drift", 0, "deriva do cristal (ppm)")
	var clients []simulator.ClientCred
	fs.Func("client", "cliente de client credentials id:arquivo_do_segredo:ok|invalido (repetível; o primeiro ok é o principal)", func(s string) error {
		c, err := simulator.ParseClientCred(s)
		clients = append(clients, c)
		return err
	})
	tokenFile := fs.String("token-file", "", "arquivo do token emitido (client-auth grava; client-auth-rotation lê)")
	metricsURL := fs.String("metrics-url", "http://api:9090/metricas", "/metricas da API (client-auth)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, _ := config.Load()
	log := newLogger(cfg)
	prefix := ""
	if *dbEnv == "SIM_BANCO_URL" {
		prefix = "SIM_"
	}
	pool, err := poolFromEnv(ctx, prefix, log)
	if err != nil {
		return err
	}
	defer pool.Close()
	q := dbgen.New(pool)

	if *keys == "" {
		return fmt.Errorf("informe --keys (CSV gerado por `tccelta-kms provisionar`)")
	}
	ks, err := simulator.ReadKeysCSV(*keys)
	if err != nil {
		return err
	}
	params := map[string]any{"api": *api, "dongles": *dongles, "duration": duration.String(), "tlat": tlat.String(), "speedup": *speedup}
	r := simulator.NewReport(*scenario, params)
	err = simulator.Run(ctx, *scenario, simulator.Options{API: *api, Inspect: postgres.Inspector(q), Repos: postgres.Repositories(q), Tx: postgres.NewTransactor(pool), Seeder: postgres.TripSeeder(q), Keys: ks, Report: r, Log: log, Dongles: *dongles,
		Duration: *duration, TLat: *tlat, Accumulated: *accumulated, Ramp: *ramp, Speedup: *speedup, Drift: *drift,
		Clients: clients, TokenFile: *tokenFile, MetricsURL: *metricsURL})
	if e := r.Write(*rel); e != nil && err == nil {
		err = e
	}
	return err
}
