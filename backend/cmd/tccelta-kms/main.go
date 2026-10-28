// Comando tccelta-kms: o serviço KMS (serve), a rotação da chave do JWT (jwt), o provisionamento de
// dongles no Vault (provision) e o healthcheck (health).
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"tccelta/backend/internal/kms/config"
)

// version é preenchida no build (-ldflags "-X main.version=...").
var version = "dev"

const usage = `tccelta-kms %s
uso: tccelta-kms <subcomando> [opções]
  serve                                    serviço KMS (HTTP, contrato em docs/kms-http-contract.md)
  jwt <rotate [--now]|schedule>            rotação da chave do JWT
  provision <new|import|import-record|export-wrapping|verify>
                                           importa K_auth/K_rec no Vault e grava record.json
  health --url <url>                       healthcheck (sai 0/1)
`

func main() {
	if len(os.Args) < 2 {
		fmt.Printf(usage, version)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	args := os.Args[2:]
	var err error
	switch os.Args[1] {
	case "serve":
		err = cmdServe(ctx)
	case "jwt":
		err = cmdJWT(ctx, args)
	case "provision":
		err = cmdProvision(ctx, args)
	case "health":
		err = cmdHealth(args)
	case "help", "-h", "--help":
		fmt.Printf(usage, version)
		return
	default:
		fmt.Fprintf(os.Stderr, "subcomando desconhecido %q\n", os.Args[1])
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
}

func newLog(c config.Config) *slog.Logger {
	var level slog.Level
	_ = level.UnmarshalText([]byte(c.LogLevel))
	op := &slog.HandlerOptions{Level: level}
	if c.LogFormat == "texto" {
		return slog.New(slog.NewTextHandler(os.Stderr, op))
	}
	return slog.New(slog.NewJSONHandler(os.Stderr, op))
}

func cmdHealth(args []string) error {
	url := "http://127.0.0.1:8300/saude/pronta"
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--url" {
			url = args[i+1]
		}
	}
	cli := &http.Client{Timeout: 3 * time.Second}
	r, err := cli.Get(url)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", r.StatusCode)
	}
	return nil
}
