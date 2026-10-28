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
	"tccelta/backend/internal/casodeuso/catalogo"
	casohs "tccelta/backend/internal/casodeuso/handshake"
	casoing "tccelta/backend/internal/casodeuso/ingestao"
	"tccelta/backend/internal/casodeuso/sessao"
	"tccelta/backend/internal/config"
	"tccelta/backend/internal/dominio/pid"
	"tccelta/backend/internal/httpapi"
	"tccelta/backend/internal/kmsremoto"
	"tccelta/backend/internal/limites"
	"tccelta/backend/internal/observabilidade/metricas"
	"tccelta/backend/internal/repositorio/cache"
	"tccelta/backend/internal/repositorio/postgres"
	"tccelta/backend/internal/token"
)

// cmdAPI é a raiz de composição da API: liga os adaptadores (postgres, kmsremoto, métricas, token)
// às portas dos casos de uso e os casos de uso ao transporte HTTP.
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

	kms, err := kmsremoto.Novo(kmsremoto.Opcoes{URL: cfg.KMS.URL, CACert: cfg.KMS.CACert, ClientCert: cfg.KMS.ClientCert,
		ClientKey: cfg.KMS.ClientKey, Timeout: cfg.KMS.Timeout, MaxConcorrencia: cfg.KMS.MaxConcorrencia}, met.PortaKMS())
	if err != nil {
		return err
	}

	// repositórios: o de dongles tem cache na frente; dentro das transações vale o do banco direto
	repos := postgres.Repositorios(consultas.New(pool))
	repos.Dongles = cache.NovoDongles(repos.Dongles, 10000, 60*time.Second)
	tx := postgres.NovoTransacionador(pool)

	cat := pid.Novo()
	sinc := catalogo.NovoSincronizar(repos.Pids, cat, log)
	if err := sinc.Recarregar(ctx); err != nil {
		return err
	}
	go sinc.Acompanhar(ctx, banco.Escutar(ctx, pool, "pid_alterado", log, nil))

	// o KMS pode demorar a subir: tenta carregar as chaves públicas do JWT por até ~1 min
	var val *token.Validador
	for tent := 0; ; tent++ {
		val, err = token.NovoValidador(ctx, kms, cfg.JWT.Emissor, cfg.JWT.Audiencia, cfg.JWT.Folga)
		if err == nil {
			break
		}
		if tent >= 30 || ctx.Err() != nil {
			return fmt.Errorf("kms: %w", err)
		}
		log.Warn("kms.indisponivel_no_boot", "erro", err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	go val.AtualizarPeriodicamente(ctx, 10*time.Minute)
	emissor := token.NovoEmissor(kms, cfg.JWT.Emissor, cfg.JWT.Audiencia, cfg.JWT.Validade)

	lim := &limites.LimitadorComFallback{Local: limites.NovoLocal(), OnFalha: func() { met.RedisFalhas.Inc() }}
	if cfg.Limites.RedisEndereco != "" {
		rc := redis.NewClient(&redis.Options{Addr: cfg.Limites.RedisEndereco, Password: cfg.Limites.RedisSenha,
			DialTimeout: cfg.Limites.RedisTimeout, ReadTimeout: cfg.Limites.RedisTimeout, WriteTimeout: cfg.Limites.RedisTimeout, MaxRetries: -1})
		lim.Redis = limites.NovoRedis(rc, cfg.Limites.RedisTimeout)
	}

	agora := func() time.Time { return time.Now() }
	paramsHS := casohs.Parametros{DesafioValidade: cfg.Protocolo.DesafioValidade, MaxPendentes: cfg.Protocolo.DesafioMaxPendentes,
		AncoraRTTMax: cfg.Protocolo.AncoraRTTMax, TLatSugerido: cfg.Protocolo.TLatSugerido, PermitirPerfilNone: cfg.PermitirPerfilNone}
	paramsIng := casoing.Parametros{LoteMax: cfg.Protocolo.LoteMaxBlocos, VerifyLote: cfg.KMS.VerifyLote,
		VerifyParalelo: cfg.KMS.VerifyParalelo, PermitirPerfilNone: cfg.PermitirPerfilNone}

	var encerrando atomic.Bool
	deps := &httpapi.Deps{
		Cfg: cfg, Log: log, Met: met.PortaHTTP(),
		PedirDesafio:     casohs.NovoEmitirDesafio(repos, kms, met.PortaHandshake(), agora, log, paramsHS),
		ResponderDesafio: casohs.NovoResponderDesafio(repos, tx, kms, emissor, met.PortaHandshake(), agora, log, paramsHS),
		ReceberLote:      casoing.NovoReceberLote(repos, tx, kms, cat, met.PortaIngestao(), agora, log, paramsIng),
		ObterConfirmacao: casoing.NovoObterConfirmacao(repos, cfg.PermitirPerfilNone),
		Autenticar:       sessao.NovoAutenticar(val, repos.Dongles, cfg.PermitirPerfilNone),
		Limitador:        lim,
		Semaforo:         limites.NovaConcorrencia(cfg.Limites.IngestaoMaxSimultaneas, cfg.Limites.IngestaoEsperaVaga),
		NaoPronto:        &encerrando,
		Prontidao: func() bool {
			c, cancel := context.WithTimeout(ctx, time.Second)
			defer cancel()
			return pool.Ping(c) == nil && kms.Pronto(c) && cat.Carregado() && val.Pronto()
		}}
	srv := &http.Server{Addr: cfg.HTTP.Endereco, Handler: deps.Roteador(), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 120 * time.Second, MaxHeaderBytes: 16 << 10}
	msrv := met.Servidor(cfg.HTTP.MetricasEndereco)

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
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 25*time.Second)
	defer cancel()
	err = srv.Shutdown(sctx)
	_ = msrv.Shutdown(sctx)
	log.Info("api.encerrada")
	return err
}
