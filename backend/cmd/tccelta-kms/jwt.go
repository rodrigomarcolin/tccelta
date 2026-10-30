package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"tccelta/backend/internal/kms/config"
	"tccelta/backend/internal/kms/vault"
)

func serveHealth(ctx context.Context, addr string) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /saude/viva", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { <-ctx.Done(); _ = srv.Close() }()
	_ = srv.ListenAndServe()
}

// cmdJWT: rotação e expurgo da chave Ed25519 do JWT no Transit.
func cmdJWT(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("uso: jwt <rotate [--now]|schedule>")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := newLog(cfg)
	cli, err := vault.NewClient(ctx, cfg.Vault, log)
	if err != nil {
		return err
	}
	defer cli.Close()
	rot := &vault.Rotator{T: cli, Mount: cfg.Vault.MountJWT, Key: cfg.Vault.JWTKey, Period: cfg.Rotation.Period,
		Validity: cfg.JWT.Validity + cfg.JWT.Leeway + 10*time.Minute, Log: log}
	switch args[0] {
	case "rotate":
		force := len(args) > 1 && args[1] == "--now"
		rotated, min, err := rot.Check(ctx, force)
		fmt.Printf("rotacionou=%v min_decryption_version=%d\n", rotated, min)
		return err
	case "schedule":
		go serveHealth(ctx, cfg.HTTP.RotationHealth)
		rot.Schedule(ctx, cfg.Rotation.CheckInterval)
		return nil
	}
	return fmt.Errorf("subcomando desconhecido %q", args[0])
}
