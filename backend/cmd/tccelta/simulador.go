package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"tccelta/backend/internal/config"
	"tccelta/backend/internal/simulador"
)

func cmdSimulador(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("simulador", flag.ContinueOnError)
	cenario := fs.String("cenario", "e2e", "e2e|protocolo-negativo|tempo|geracao|carga|reconexao|joelho")
	api := fs.String("api", "http://api:8080", "URL base da API")
	chaves := fs.String("chaves", "", "CSV de dongles de teste")
	dongles := fs.Int("dongles", 0, "número de dongles virtuais")
	duracao := fs.Duration("duracao", 2*time.Minute, "duração da carga")
	acelerar := fs.Float64("acelerar", 1, "fator de aceleração do T_lat")
	tlat := fs.Duration("tlat", 60*time.Second, "T_lat")
	acumulado := fs.Int("acumulado", 500, "blocos acumulados offline")
	rampa := fs.Duration("rampa", 30*time.Second, "rampa de conexão")
	bancoEnv := fs.String("banco-url-env", "SIM_BANCO_URL", "variável com a URL do banco (SELECT)")
	rel := fs.String("relatorio", "", "arquivo do relatório JSON")
	deriva := fs.Float64("deriva", 0, "deriva do cristal (ppm)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, _ := config.Carregar()
	log := novoLog(cfg)
	prefixo := ""
	if *bancoEnv == "SIM_BANCO_URL" {
		prefixo = "SIM_"
	}
	pool, err := poolDeEnv(ctx, prefixo, log)
	if err != nil {
		return err
	}
	defer pool.Close()

	if *chaves == "" {
		return fmt.Errorf("informe --chaves (CSV gerado por `tccelta-kms provisionar`)")
	}
	ks, err := lerCSVChaves(*chaves)
	if err != nil {
		return err
	}
	params := map[string]any{"api": *api, "dongles": *dongles, "duracao": duracao.String(), "tlat": tlat.String(), "acelerar": *acelerar}
	r := simulador.NovoRelatorio(*cenario, params)
	err = simulador.Executar(ctx, *cenario, simulador.Opcoes{API: *api, Pool: pool, Chaves: ks, Rel: r, Log: log, Dongles: *dongles,
		Duracao: *duracao, TLat: *tlat, Acumulado: *acumulado, Rampa: *rampa, Acelerar: *acelerar, Deriva: *deriva})
	if e := r.Escrever(*rel); e != nil && err == nil {
		err = e
	}
	return err
}
