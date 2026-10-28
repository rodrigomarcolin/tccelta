package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"tccelta/backend/internal/kms/config"
	"tccelta/backend/internal/kms/vault"
)

func servirSaude(ctx context.Context, addr string) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /saude/viva", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { <-ctx.Done(); _ = srv.Close() }()
	_ = srv.ListenAndServe()
}

// cmdJWT: rotação e expurgo da chave Ed25519 do JWT no Transit.
func cmdJWT(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("uso: jwt <rotacionar [--agora]|agendar>")
	}
	cfg, err := config.Carregar()
	if err != nil {
		return err
	}
	log := novoLog(cfg)
	cli, err := vault.NovoCliente(ctx, cfg.Vault, log)
	if err != nil {
		return err
	}
	defer cli.Fechar()
	rot := &vault.Rotacionador{T: cli, Mount: cfg.Vault.MountJWT, Chave: cfg.Vault.ChaveJWT, Periodo: cfg.Rotacao.Periodo,
		Validade: cfg.JWT.Validade + cfg.JWT.Folga + 10*time.Minute, Log: log}
	switch args[0] {
	case "rotacionar":
		forcar := len(args) > 1 && args[1] == "--agora"
		rotacionou, min, err := rot.Verificar(ctx, forcar)
		fmt.Printf("rotacionou=%v min_decryption_version=%d\n", rotacionou, min)
		return err
	case "agendar":
		go servirSaude(ctx, cfg.HTTP.SaudeRotacao)
		rot.Agendar(ctx, cfg.Rotacao.Verificacao)
		return nil
	}
	return fmt.Errorf("subcomando desconhecido %q", args[0])
}
