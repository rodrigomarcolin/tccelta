// Comando tccelta-kms: o serviço KMS (servir), a rotação da chave do JWT (jwt), o provisionamento de
// dongles no Vault (provisionar) e o healthcheck (saude).
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

// versao é preenchida no build (-ldflags "-X main.versao=...").
var versao = "dev"

const ajuda = `tccelta-kms %s
uso: tccelta-kms <subcomando> [opções]
  servir                                   serviço KMS (HTTP, contrato em docs/contrato-http.md)
  jwt <rotacionar [--agora]|agendar>       rotação da chave do JWT
  provisionar <novo|importar|importar-registro|exportar-wrapping|verificar>
                                           importa K_auth/K_rec no Vault e grava registro.json
  saude --url <url>                        healthcheck (sai 0/1)
`

func main() {
	if len(os.Args) < 2 {
		fmt.Printf(ajuda, versao)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	args := os.Args[2:]
	var err error
	switch os.Args[1] {
	case "servir":
		err = cmdServir(ctx)
	case "jwt":
		err = cmdJWT(ctx, args)
	case "provisionar":
		err = cmdProvisionar(ctx, args)
	case "saude":
		err = cmdSaude(args)
	case "ajuda", "-h", "--help":
		fmt.Printf(ajuda, versao)
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

func novoLog(c config.Config) *slog.Logger {
	var nivel slog.Level
	_ = nivel.UnmarshalText([]byte(c.LogNivel))
	op := &slog.HandlerOptions{Level: nivel}
	if c.LogFormato == "texto" {
		return slog.New(slog.NewTextHandler(os.Stderr, op))
	}
	return slog.New(slog.NewJSONHandler(os.Stderr, op))
}

func cmdSaude(args []string) error {
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
