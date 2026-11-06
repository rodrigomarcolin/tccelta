package http

import (
	"log/slog"
	"net/http"
	"sync/atomic"

	"tccelta/backend/internal/config"
	"tccelta/backend/internal/domain/protocol"
	"tccelta/backend/internal/http/ratelimit"
	"tccelta/backend/internal/port"
)

// Deps são as dependências do roteador: os casos de uso, a porta de métricas e os limites.
type Deps struct {
	Cfg                config.Config
	Log                *slog.Logger
	Met                port.MetricsHTTP
	RequestChallenge   RequestChallenge
	RespondChallenge   RespondChallenge
	ReceiveBatch       ReceiveBatch
	GetConfirmation    GetConfirmation
	CreateTrip         CreateTrip
	GetTrip            GetTrip
	GetAnalysis        GetAnalysis
	Authenticate       Authenticate
	IssueToken         IssueToken
	AuthenticateClient AuthenticateClient
	ClientKeys         GetClientKeys
	MetAuth            port.MetricsAuth // pode ser nil
	Limiter            ratelimit.Limiter
	Semaphore          *ratelimit.Concurrency
	Readiness          func() bool // banco, KMS, PIDs, chave pública
	NotReady           *atomic.Bool
}

// maxTokenBody é o limite do corpo de POST /v1/auth/token (só grant_type=client_credentials).
const maxTokenBody = 1 << 10

// Router monta as rotas e a cadeia de middlewares.
func (d *Deps) Router() http.Handler {
	mux := http.NewServeMux()
	chain := func(h http.Handler, ms ...mw) http.Handler {
		for i := len(ms) - 1; i >= 0; i-- {
			h = ms[i](h)
		}
		return h
	}
	maxBatch := int64(d.Cfg.Protocol.MaxBatchBlocks*protocol.MaxBlockTLVSize + 64)
	base := func(route string, h http.HandlerFunc, ms ...mw) http.Handler {
		return chain(h, append([]mw{recoverPanics(d.Log), d.instrument(route)}, ms...)...)
	}
	mux.Handle("POST /v1/dongles/{id}/desafio", base("challenge", d.requestChallenge,
		d.limitIP, d.limitDevID("challenge", d.Cfg.Limits.ChallengeMin)))
	mux.Handle("POST /v1/dongles/{id}/desafio/resposta", base("response", d.respondChallenge,
		annotateS2, d.limitIP, d.limitDevID("response", d.Cfg.Limits.ResponseMin)))
	mux.Handle("POST /v1/dongles/blocos", base("blocks", d.receiveBatch,
		d.limitIP, d.authenticateJWT(false), d.limitDevID("blocks", d.Cfg.Limits.BlocksMin), d.limitConcurrency, limitBody(maxBatch)))
	mux.Handle("GET /v1/dongles/{id}/confirmacao", base("confirmation", d.getConfirmation,
		d.limitIP, d.authenticateJWT(true), d.limitDevID("confirmation", 60)))
	// Rotas de viagem: exigem o Bearer de client credentials.
	vip := d.limitIPScope("viagem_ip", d.Cfg.Limits.TripIPMin)
	tripAuth := func(ms ...mw) []mw { return append(ms, d.authenticateAnalise) }
	mux.Handle("POST /v1/viagens", base("trip_create", d.createTrip, tripAuth(vip, limitBody(maxTripBody))...))
	mux.Handle("GET /v1/viagens/{id}", base("trip_status", d.getTrip, tripAuth(vip)...))
	mux.Handle("GET /v1/viagens/{id}/analise", base("trip_analysis", d.getAnalysis, tripAuth(vip)...))
	// Client credentials e chaves públicas.
	mux.Handle("POST /v1/auth/token", base("auth_token", d.issueToken, noStore, d.limitIP, limitBody(maxTokenBody)))
	mux.Handle("GET /.well-known/jwks.json", base("jwks", d.jwks, d.limitIP))
	mux.HandleFunc("GET /saude/viva", d.live)
	mux.HandleFunc("GET /saude/pronta", d.ready)
	return mux
}
