// Comando tccelta: binário único com os subcomandos api, worker, migrar, provisionar (registrar), simulador e saude.
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

	"tccelta/backend/internal/banco"
	"tccelta/backend/internal/config"
	"tccelta/backend/internal/versao"
)

const ajuda = `tccelta ` + "%s" + `
uso: tccelta <subcomando> [opções]
  api                          servidor HTTP
  worker                       worker de tempo; "worker reprocessar" refaz leitura/posicao
  migrar [up|down-to N|status] migrations
  provisionar registrar --arquivo registro.json  cadastra no banco um dongle provisionado pelo KMS
  simulador                    dongles virtuais e cenários
  saude --url <url>            healthcheck (sai 0/1)
`

func main() {
	if len(os.Args) < 2 {
		fmt.Printf(ajuda, versao.Versao)
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
	case "migrar":
		err = cmdMigrar(ctx, args)
	case "provisionar":
		err = cmdProvisionar(ctx, args)
	case "simulador":
		err = cmdSimulador(ctx, args)
	case "saude":
		err = cmdSaude(args)
	case "ajuda", "-h", "--help":
		fmt.Printf(ajuda, versao.Versao)
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

// poolDeEnv abre um pool a partir de <PREFIXO>BANCO_URL / <PREFIXO>BANCO_SENHA_ARQUIVO.
func poolDeEnv(ctx context.Context, prefixo string, log *slog.Logger) (*pgxpool.Pool, error) {
	url := os.Getenv(prefixo + "BANCO_URL")
	if url == "" {
		return nil, fmt.Errorf("%sBANCO_URL não definida", prefixo)
	}
	senha := ""
	if arq := os.Getenv(prefixo + "BANCO_SENHA_ARQUIVO"); arq != "" {
		b, err := os.ReadFile(arq)
		if err != nil {
			return nil, err
		}
		senha = strings.TrimSpace(string(b))
	}
	return banco.NovoPool(ctx, config.BancoConfig{URL: url, Senha: senha, PoolMax: 10, PoolMin: 1}, log)
}

func cmdSaude(args []string) error {
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
