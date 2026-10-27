package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"

	"tccelta/backend/internal/banco"
	"tccelta/backend/internal/banco/consultas"
	"tccelta/backend/internal/config"
	"tccelta/backend/internal/dongles"
	"tccelta/backend/internal/handshake"
	"tccelta/backend/internal/httpapi"
	"tccelta/backend/internal/ingestao"
	"tccelta/backend/internal/kms"
	"tccelta/backend/internal/limites"
	"tccelta/backend/internal/metricas"
	"tccelta/backend/internal/pids"
	"tccelta/backend/internal/token"
)

func cmdAPI(ctx context.Context) error {
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

	// Vault: login AppRole com novas tentativas (o Vault pode demorar a subir)
	var vault *kms.Vault
	for tent := 0; ; tent++ {
		vault, err = kms.NovoVault(ctx, cfg.Vault, log, met)
		if err == nil {
			break
		}
		if tent >= 30 || ctx.Err() != nil {
			return fmt.Errorf("vault: %w", err)
		}
		log.Warn("vault.indisponivel_no_boot", "erro", err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	defer vault.Fechar()

	q := consultas.New(pool)
	cat := pids.Novo(q, log)
	if err := cat.Recarregar(ctx); err != nil {
		return err
	}
	go cat.Acompanhar(ctx, banco.Escutar(ctx, pool, "pid_alterado", log, nil))

	val, err := token.NovoValidador(ctx, vault, cfg.JWT.Emissor, cfg.JWT.Audiencia, cfg.JWT.Folga)
	if err != nil {
		return err
	}
	go val.AtualizarPeriodicamente(ctx, 10*time.Minute)
	emissor := &token.Emissor{Assinador: vault, Emissor: cfg.JWT.Emissor, Audiencia: cfg.JWT.Audiencia, Validade: cfg.JWT.Validade}
	cache := dongles.Novo(q, 10000, 60*time.Second)

	local := limites.NovoLocal()
	lim := &limites.LimitadorComFallback{Local: local, OnFalha: func() { met.RedisFalhas.Inc() }}
	if cfg.Limites.RedisEndereco != "" {
		rc := redis.NewClient(&redis.Options{Addr: cfg.Limites.RedisEndereco, Password: cfg.Limites.RedisSenha,
			DialTimeout: cfg.Limites.RedisTimeout, ReadTimeout: cfg.Limites.RedisTimeout, WriteTimeout: cfg.Limites.RedisTimeout, MaxRetries: -1})
		lim.Redis = limites.NovoRedis(rc, cfg.Limites.RedisTimeout)
	}

	hs := &handshake.Servico{Pool: pool, KMS: vault, Tokens: emissor, Cfg: cfg.Protocolo, Log: log, Met: met}
	ing := &ingestao.Servico{Pool: pool, KMS: vault, Pids: cat, LoteMax: cfg.Protocolo.LoteMaxBlocos,
		VerifyLote: cfg.Vault.VerifyLote, VerifyParalelo: cfg.Vault.VerifyParalelo, Log: log, Met: met}

	var encerrando atomic.Bool
	deps := &httpapi.Deps{Cfg: cfg, Log: log, Met: met, Handshake: hs, Ingestao: ing, Dongles: cache, Validador: val,
		Limitador: lim, Semaforo: limites.NovaConcorrencia(cfg.Limites.IngestaoMaxSimultaneas, cfg.Limites.IngestaoEsperaVaga),
		NaoPronto: &encerrando,
		Prontidao: func() bool {
			c, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			return pool.Ping(c) == nil && vault.Pronto() && cat.Carregado() && val.Pronto()
		}}
	srv := &http.Server{Addr: cfg.HTTP.Endereco, Handler: deps.Roteador(), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 120 * time.Second, MaxHeaderBytes: 16 << 10}
	mux := http.NewServeMux()
	mux.Handle("GET /metricas", met.Handler())
	msrv := &http.Server{Addr: cfg.HTTP.MetricasEndereco, Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	erros := make(chan error, 2)
	go func() { erros <- srv.ListenAndServe() }()
	go func() { erros <- msrv.ListenAndServe() }()
	log.Info("api.iniciada", "endereco", cfg.HTTP.Endereco)
	select {
	case err := <-erros:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	encerrando.Store(true) // /saude/pronta => 503: o Caddy tira a réplica do balanceamento
	time.Sleep(2 * time.Second)
	sctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	err = srv.Shutdown(sctx)
	_ = msrv.Shutdown(sctx)
	log.Info("api.encerrada")
	return err
}
