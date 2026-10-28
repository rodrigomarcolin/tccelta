// Comando tccelta: binário único com os subcomandos api, worker, migrate, provision (register), simulator e health.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"tccelta/backend/internal/config"
	"tccelta/backend/internal/database"
	"tccelta/backend/internal/version"
)

const help = `tccelta ` + "%s" + `
uso: tccelta <subcomando> [opções]
  api                          servidor HTTP
  worker                       worker de tempo; "worker reprocess" refaz reading/position
  migrate [up|down-to N|status] migrations
  provision register --file registro.json  cadastra no banco um dongle provisionado pelo KMS
  simulator                    dongles virtuais e cenários
  health --url <url>           healthcheck (sai 0/1)
`

func main() {
	if len(os.Args) < 2 {
		fmt.Printf(help, version.Version)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	args := os.Args[2:]
	var err error
	switch os.Args[1] {
	case "api":
		err = cmdAPI(ctx)
	case "worker":
		err = cmdWorker(ctx, args)
	case "migrate":
		err = cmdMigrate(ctx, args)
	case "provision":
		err = cmdProvision(ctx, args)
	case "simulator":
		err = cmdSimulator(ctx, args)
	case "health":
		err = cmdHealth(args)
	case "help", "-h", "--help":
		fmt.Printf(help, version.Version)
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

func newLogger(c config.Config) *slog.Logger {
	var level slog.Level
	_ = level.UnmarshalText([]byte(c.LogLevel))
	op := &slog.HandlerOptions{Level: level}
	if c.LogFormat == "texto" {
		return slog.New(slog.NewTextHandler(os.Stderr, op))
	}
	return slog.New(slog.NewJSONHandler(os.Stderr, op))
}

// poolFromEnv abre um pool a partir de <PREFIXO>BANCO_URL / <PREFIXO>BANCO_SENHA_ARQUIVO.
func poolFromEnv(ctx context.Context, prefix string, log *slog.Logger) (*pgxpool.Pool, error) {
	url := os.Getenv(prefix + "BANCO_URL")
	if url == "" {
		return nil, fmt.Errorf("%sBANCO_URL não definida", prefix)
	}
	password := ""
	if file := os.Getenv(prefix + "BANCO_SENHA_ARQUIVO"); file != "" {
		b, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		password = strings.TrimSpace(string(b))
	}
	return database.NewPool(ctx, config.DatabaseConfig{URL: url, Password: password, PoolMax: 10, PoolMin: 1}, log)
}

func cmdHealth(args []string) error {
	url := "http://127.0.0.1:8080/saude/pronta"
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
