package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"tccelta/kms/internal/config"
	"tccelta/kms/internal/metricas"
	"tccelta/kms/internal/servidor"
	"tccelta/kms/internal/vault"
)

// cmdServir sobe o serviço KMS: login no Vault (com novas tentativas), servidor HTTP (mTLS fora do
// ambiente dev) e servidor de métricas em porta separada.
func cmdServir(ctx context.Context) error {
	cfg, err := config.Carregar()
	if err != nil {
		return err
	}
	if err := cfg.ValidarServidor(); err != nil {
		return err
	}
	log := novoLog(cfg)
	met := metricas.Nova()

	var v *vault.Vault
	for tent := 0; ; tent++ {
		v, err = vault.NovoVault(ctx, cfg.Vault, log, met)
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
	defer v.Fechar()

	s := &servidor.Servidor{Vault: v, Log: log, Met: met, ExigirCertCliente: !cfg.Dev()}
	srv := &http.Server{Addr: cfg.HTTP.Endereco, Handler: s.Roteador(), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 120 * time.Second, MaxHeaderBytes: 16 << 10}
	tls := cfg.HTTP.TLSCert != ""
	if tls {
		if srv.TLSConfig, err = servidor.ConfigTLS(cfg.HTTP.TLSCert, cfg.HTTP.TLSKey, cfg.HTTP.TLSClientCA); err != nil {
			return err
		}
	}
	msrv := met.Servidor(cfg.HTTP.MetricasEndereco)
	s.RotasSaude(msrv.Handler.(*http.ServeMux))

	erros := make(chan error, 2)
	go func() {
		if tls {
			erros <- srv.ListenAndServeTLS("", "")
		} else {
			erros <- srv.ListenAndServe()
		}
	}()
	go func() { erros <- msrv.ListenAndServe() }()
	log.Info("kms.iniciado", "endereco", cfg.HTTP.Endereco, "mtls", tls, "versao", versao)
	select {
	case err := <-erros:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	sctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	err = srv.Shutdown(sctx)
	_ = msrv.Shutdown(sctx)
	log.Info("kms.encerrado")
	return err
}
