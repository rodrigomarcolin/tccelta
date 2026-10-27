// Package servidor expõe o KMS por HTTP (contrato em docs/contrato-http.md). Um arquivo por endpoint;
// aqui ficam a tabela de rotas, as dependências e os middlewares comuns.
package servidor

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"tccelta/kms/internal/metricas"
)

// Vault é o que o servidor precisa das operações sobre o Transit.
type Vault interface {
	HMACAuth(ctx context.Context, chave string, entradas [][]byte) ([][]byte, error)
	VerificarBlocos(ctx context.Context, chave string, entradas, tags [][]byte) ([]bool, error)
	Assinar(ctx context.Context, entrada []byte) (assinatura []byte, versao int, err error)
	ChavesPublicas(ctx context.Context) (map[int]ed25519.PublicKey, error)
	// Pronto: token do Vault válido e disjuntor fechado.
	Pronto() bool
}

// Limites do contrato.
const (
	// MaxItens é o máximo de entradas por requisição de lote (a API envia até 32 por chamada).
	MaxItens = 256
	// MaxCorpo é o tamanho máximo do corpo JSON.
	MaxCorpo = 1 << 20
	// MaxEntrada é o tamanho máximo de cada entrada decodificada.
	MaxEntrada = 4096
)

// Servidor liga o contrato HTTP ao Vault.
type Servidor struct {
	Vault Vault
	Log   *slog.Logger
	Met   *metricas.M
	// ExigirCertCliente recusa requisições sem certificado de cliente verificado (defesa em
	// profundidade além do tls.Config; ligado fora do ambiente dev).
	ExigirCertCliente bool
}

type mw = func(http.Handler) http.Handler

// Roteador monta as rotas e a cadeia de middlewares.
func (s *Servidor) Roteador() http.Handler {
	mux := http.NewServeMux()
	base := func(rota string, h http.HandlerFunc) http.Handler {
		var c http.Handler = h
		for _, m := range []mw{limitarCorpo(MaxCorpo), s.exigirCliente, s.instrumentar(rota), recuperar(s.Log)} {
			c = m(c)
		}
		return c
	}
	mux.Handle("POST /v1/hmac", base("hmac", s.hmac))
	mux.Handle("POST /v1/blocos/verificar", base("verificar", s.verificarBlocos))
	mux.Handle("POST /v1/jwt/assinar", base("assinar", s.assinarJWT))
	mux.Handle("GET /v1/jwt/chaves", base("chaves", s.chavesPublicas))
	s.RotasSaude(mux)
	return mux
}

type respWriter struct {
	http.ResponseWriter
	status int
}

func (r *respWriter) WriteHeader(c int) { r.status = c; r.ResponseWriter.WriteHeader(c) }

func recuperar(log *slog.Logger) mw {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if x := recover(); x != nil {
					log.Error("http.panic", "erro", fmt.Sprint(x), "pilha", string(debug.Stack()))
					problema(w, 500, "erro_interno", "erro interno", 0)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// instrumentar: X-Request-Id, log de acesso e métricas HTTP.
func (s *Servidor) instrumentar(rota string) mw {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t0 := time.Now()
			id := r.Header.Get("X-Request-Id")
			if id == "" {
				id = novoID()
			}
			w.Header().Set("X-Request-Id", id)
			rw := &respWriter{ResponseWriter: w, status: 200}
			next.ServeHTTP(rw, r)
			dur := time.Since(t0)
			if s.Met != nil {
				s.Met.Requisicao(rota, rw.status, dur)
			}
			s.Log.Info("http.acesso", "req_id", id, "rota", rota, "status", rw.status, "duracao_ms", dur.Milliseconds())
		})
	}
}

func limitarCorpo(max int64) mw {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, max)
			next.ServeHTTP(w, r)
		})
	}
}

func novoID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
