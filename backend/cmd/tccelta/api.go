package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"

	"tccelta/backend/internal/config"
	"tccelta/backend/internal/database"
	"tccelta/backend/internal/database/dbgen"
	"tccelta/backend/internal/domain/pid"
	"tccelta/backend/internal/httpapi"
	"tccelta/backend/internal/limits"
	"tccelta/backend/internal/observability/metrics"
	"tccelta/backend/internal/remotekms"
	"tccelta/backend/internal/repository/cache"
	"tccelta/backend/internal/repository/postgres"
	"tccelta/backend/internal/token"
	"tccelta/backend/internal/usecase/catalog"
	hsUC "tccelta/backend/internal/usecase/handshake"
	ingUC "tccelta/backend/internal/usecase/ingestion"
	"tccelta/backend/internal/usecase/session"
)

// cmdAPI é a raiz de composição da API: liga os adaptadores (postgres, remotekms, métricas, token)
// às portas dos casos de uso e os casos de uso ao transporte HTTP.
func cmdAPI(ctx context.Context) error {
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
	met.RegisterPool(pool)

	kms, err := remotekms.New(remotekms.Options{URL: cfg.KMS.URL, CACert: cfg.KMS.CACert, ClientCert: cfg.KMS.ClientCert,
		ClientKey: cfg.KMS.ClientKey, Timeout: cfg.KMS.Timeout, MaxConcurrency: cfg.KMS.MaxConcurrency}, met.KMSPort())
	if err != nil {
		return err
	}

	// repositórios: o de dongles tem cache na frente; dentro das transações vale o do banco direto
	repos := postgres.Repositories(dbgen.New(pool))
	repos.Dongles = cache.NewDongles(repos.Dongles, 10000, 60*time.Second)
	tx := postgres.NewTransactor(pool)

	cat := pid.New()
	sinc := catalog.NewSync(repos.Pids, cat, log)
	if err := sinc.Reload(ctx); err != nil {
		return err
	}
	go sinc.Track(ctx, database.Listen(ctx, pool, "pid_changed", log, nil))

	// o KMS pode demorar a subir: tenta carregar as chaves públicas do JWT por até ~1 min
	var val *token.Validator
	for attempt := 0; ; attempt++ {
		val, err = token.NewValidator(ctx, kms, cfg.JWT.Issuer, cfg.JWT.Audience, cfg.JWT.Leeway)
		if err == nil {
			break
		}
		if attempt >= 30 || ctx.Err() != nil {
			return fmt.Errorf("kms: %w", err)
		}
		log.Warn("kms.indisponivel_no_boot", "erro", err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	go val.UpdatePeriodically(ctx, 10*time.Minute)
	issuer := token.NewIssuer(kms, cfg.JWT.Issuer, cfg.JWT.Audience, cfg.JWT.Validity)

	lim := &limits.LimiterWithFallback{Local: limits.NewLocal(), OnFailure: func() { met.RedisFailures.Inc() }}
	if cfg.Limits.RedisAddress != "" {
		rc := redis.NewClient(&redis.Options{Addr: cfg.Limits.RedisAddress, Password: cfg.Limits.RedisPassword,
			DialTimeout: cfg.Limits.RedisTimeout, ReadTimeout: cfg.Limits.RedisTimeout, WriteTimeout: cfg.Limits.RedisTimeout, MaxRetries: -1})
		lim.Redis = limits.NewRedis(rc, cfg.Limits.RedisTimeout)
	}

	now := func() time.Time { return time.Now() }
	paramsHS := hsUC.Params{ChallengeValidity: cfg.Protocol.ChallengeValidity, MaxPending: cfg.Protocol.MaxPendingChallenges,
		AnchorRTTMax: cfg.Protocol.AnchorRTTMax, SuggestedTLat: cfg.Protocol.SuggestedTLat, AllowNoneProfile: cfg.AllowNoneProfile}
	paramsIng := ingUC.Params{MaxBatch: cfg.Protocol.MaxBatchBlocks, VerifyBatch: cfg.KMS.VerifyBatch,
		VerifyParallel: cfg.KMS.VerifyParallel, AllowNoneProfile: cfg.AllowNoneProfile}

	var closing atomic.Bool
	deps := &httpapi.Deps{
		Cfg: cfg, Log: log, Met: met.HTTPPort(),
		RequestChallenge: hsUC.NewIssueChallenge(repos, kms, met.HandshakePort(), now, log, paramsHS),
		RespondChallenge: hsUC.NewRespondChallenge(repos, tx, kms, issuer, met.HandshakePort(), now, log, paramsHS),
		ReceiveBatch:     ingUC.NewReceiveBatch(repos, tx, kms, cat, met.IngestionPort(), now, log, paramsIng),
		GetConfirmation:  ingUC.NewGetConfirmation(repos, cfg.AllowNoneProfile),
		Authenticate:     session.NewAuthenticate(val, repos.Dongles, cfg.AllowNoneProfile),
		Limiter:          lim,
		Semaphore:        limits.NewConcurrency(cfg.Limits.MaxConcurrentIngestions, cfg.Limits.IngestionSlotWait),
		NotReady:         &closing,
		Readiness: func() bool {
			c, cancel := context.WithTimeout(ctx, time.Second)
			defer cancel()
			return pool.Ping(c) == nil && kms.Ready(c) && cat.Loaded() && val.Ready()
		}}
	srv := &http.Server{Addr: cfg.HTTP.Address, Handler: deps.Router(), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 120 * time.Second, MaxHeaderBytes: 16 << 10}
	msrv := met.Server(cfg.HTTP.MetricsAddress)

	errs := make(chan error, 2)
	go func() { errs <- srv.ListenAndServe() }()
	go func() { errs <- msrv.ListenAndServe() }()
	log.Info("api.iniciada", "endereco", cfg.HTTP.Address)
	select {
	case err := <-errs:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	closing.Store(true) // /saude/pronta => 503: o Caddy tira a réplica do balanceamento
	time.Sleep(2 * time.Second)
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 25*time.Second)
	defer cancel()
	err = srv.Shutdown(sctx)
	_ = msrv.Shutdown(sctx)
	log.Info("api.encerrada")
	return err
}
