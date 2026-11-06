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

// cmdJWT: rotação e expurgo da chave Ed25519 do JWT do app no Transit.
func cmdJWT(ctx context.Context, args []string) error {
	return rotateKey(ctx, "jwt", args, func(c config.Config) keyPlan {
		return keyPlan{key: c.Vault.JWTKey, period: c.Rotation.Period, validity: c.JWT.Validity + c.JWT.Leeway + 10*time.Minute}
	})
}

// cmdClientJWT: rotação e expurgo da chave Ed25519 `cli` (JWT dos clientes) no mesmo mount do Transit.
func cmdClientJWT(ctx context.Context, args []string) error {
	return rotateKey(ctx, "clientjwt", args, func(c config.Config) keyPlan {
		return keyPlan{key: c.Vault.ClientJWTKey, period: c.ClientJWT.RotationPeriod, validity: c.ClientJWT.Validity + c.JWT.Leeway + 10*time.Minute}
	})
}

// keyPlan diz o que girar: o nome da chave no mount do JWT, a cada quanto tempo e por quanto tempo uma versão
// antiga ainda precisa validar tokens.
type keyPlan struct {
	key              string
	period, validity time.Duration
}

func rotateKey(ctx context.Context, cmd string, args []string, plan func(config.Config) keyPlan) error {
	if len(args) == 0 {
		return fmt.Errorf("uso: %s <rotate [--now]|schedule>", cmd)
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
	p := plan(cfg)
	rot := &vault.Rotator{T: cli, Mount: cfg.Vault.MountJWT, Key: p.key, Period: p.period, Validity: p.validity, Log: log}
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
