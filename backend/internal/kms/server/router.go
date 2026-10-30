// Package server expõe o KMS por HTTP (contrato em docs/kms-http-contract.md). Um arquivo por endpoint;
// aqui ficam a tabela de rotas, as dependências e os middlewares comuns.
package server

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

	"tccelta/backend/internal/kms/metrics"
)

// Vault é o que o servidor precisa das operações sobre o Transit.
type Vault interface {
	HMACAuth(ctx context.Context, key string, inputs [][]byte) ([][]byte, error)
	VerifyBlocks(ctx context.Context, key string, inputs, tags [][]byte) ([]bool, error)
	Sign(ctx context.Context, input []byte) (signature []byte, version int, err error)
	PublicKeys(ctx context.Context) (map[int]ed25519.PublicKey, error)
	// Ready: token do Vault válido e disjuntor fechado.
	Ready() bool
}

// Limites do contrato.
const (
	// MaxItems é o máximo de entradas por requisição de lote (a API envia até 32 por chamada).
	MaxItems = 256
	// MaxBody é o tamanho máximo do corpo JSON.
	MaxBody = 1 << 20
	// MaxInput é o tamanho máximo de cada entrada decodificada.
	MaxInput = 4096
)

// Server liga o contrato HTTP ao Vault.
type Server struct {
	Vault   Vault
	Log     *slog.Logger
	Metrics *metrics.M
	// RequireClientCert recusa requisições sem certificado de cliente verificado (defesa em
	// profundidade além do tls.Config; ligado fora do ambiente dev).
	RequireClientCert bool
	// ClientCN é o CN exigido no certificado folha do cliente (com ExigirCertCliente). Impede que
	// outro certificado emitido pela mesma CA use o KMS.
	ClientCN string
}

type mw = func(http.Handler) http.Handler

// Router monta as rotas e a cadeia de middlewares.
func (s *Server) Router() http.Handler {
	mux := http.NewServeMux()
	base := func(route string, h http.HandlerFunc) http.Handler {
		var c http.Handler = h
		for _, m := range []mw{limitBody(MaxBody), s.requireClient, s.instrument(route), recoverPanic(s.Log)} {
			c = m(c)
		}
		return c
	}
	mux.Handle("POST /v1/hmac", base("hmac", s.hmac))
	mux.Handle("POST /v1/blocks/verify", base("verify", s.verifyBlocks))
	mux.Handle("POST /v1/jwt/sign", base("sign", s.signJWT))
	mux.Handle("GET /v1/jwt/keys", base("keys", s.publicKeys))
	s.HealthRoutes(mux)
	return mux
}

type respWriter struct {
	http.ResponseWriter
	status int
}

func (r *respWriter) WriteHeader(c int) { r.status = c; r.ResponseWriter.WriteHeader(c) }

func recoverPanic(log *slog.Logger) mw {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if x := recover(); x != nil {
					log.Error("http.panic", "erro", fmt.Sprint(x), "pilha", string(debug.Stack()))
					problem(w, 500, "erro_interno", "erro interno", 0)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// instrument: X-Request-Id, log de acesso e métricas HTTP.
func (s *Server) instrument(route string) mw {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t0 := time.Now()
			id := r.Header.Get("X-Request-Id")
			if id == "" {
				id = newID()
			}
			w.Header().Set("X-Request-Id", id)
			rw := &respWriter{ResponseWriter: w, status: 200}
			next.ServeHTTP(rw, r)
			dur := time.Since(t0)
			if s.Metrics != nil {
				s.Metrics.HTTPRequest(route, rw.status, dur)
			}
			s.Log.Info("http.acesso", "req_id", id, "rota", route, "status", rw.status, "duracao_ms", dur.Milliseconds())
		})
	}
}

func limitBody(max int64) mw {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, max)
			next.ServeHTTP(w, r)
		})
	}
}

func newID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
