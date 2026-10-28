package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"tccelta/backend/internal/kms/config"
	"tccelta/backend/internal/kms/metrics"
	"tccelta/backend/internal/kms/server"
	"tccelta/backend/internal/kms/vault"
)

// cmdServe sobe o serviço KMS: login no Vault (com novas tentativas), servidor HTTP (mTLS fora do
// environment dev) e servidor de métricas em porta separada.
func cmdServe(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := cfg.ValidateServer(); err != nil {
		return err
	}
	log := newLog(cfg)
	met := metrics.New()

	var v *vault.Vault
	for attempt := 0; ; attempt++ {
		v, err = vault.NewVault(ctx, cfg.Vault, log, met)
		if err == nil {
			break
		}
		if attempt >= 30 || ctx.Err() != nil {
			return fmt.Errorf("vault: %w", err)
		}
		log.Warn("vault.indisponivel_no_boot", "erro", err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	defer v.Close()

	s := &server.Server{Vault: v, Log: log, Metrics: met, RequireClientCert: !cfg.Dev(), ClientCN: cfg.HTTP.TLSClientCN}
	srv := &http.Server{Addr: cfg.HTTP.Address, Handler: s.Router(), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 120 * time.Second, MaxHeaderBytes: 16 << 10}
	tls := cfg.HTTP.TLSCert != ""
	if tls {
		if srv.TLSConfig, err = server.ConfigTLS(cfg.HTTP.TLSCert, cfg.HTTP.TLSKey, cfg.HTTP.TLSClientCA); err != nil {
			return err
		}
	}
	msrv := met.Server(cfg.HTTP.MetricsAddress)
	s.HealthRoutes(msrv.Handler.(*http.ServeMux))

	errc := make(chan error, 2)
	go func() {
		if tls {
			errc <- srv.ListenAndServeTLS("", "")
		} else {
			errc <- srv.ListenAndServe()
		}
	}()
	go func() { errc <- msrv.ListenAndServe() }()
	log.Info("kms.iniciado", "endereco", cfg.HTTP.Address, "mtls", tls, "version", version)
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
	defer cancel()
	err = srv.Shutdown(sctx)
	_ = msrv.Shutdown(sctx)
	log.Info("kms.encerrado")
	return err
}
