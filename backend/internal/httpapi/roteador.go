package httpapi

import (
	"log/slog"
	"net/http"
	"sync/atomic"

	"tccelta/backend/internal/casodeuso/porta"
	"tccelta/backend/internal/config"
	"tccelta/backend/internal/limites"
)

// Deps são as dependências do roteador: os casos de uso, a porta de métricas e os limites.
type Deps struct {
	Cfg              config.Config
	Log              *slog.Logger
	Met              porta.MetricasHTTP
	PedirDesafio     PedirDesafio
	ResponderDesafio ResponderDesafio
	ReceberLote      ReceberLote
	ObterConfirmacao ObterConfirmacao
	Autenticar       Autenticar
	Limitador        limites.Limitador
	Semaforo         *limites.Concorrencia
	Prontidao        func() bool // banco, KMS, PIDs, chave pública
	NaoPronto        *atomic.Bool
}

// Roteador monta as rotas e a cadeia de middlewares.
func (d *Deps) Roteador() http.Handler {
	mux := http.NewServeMux()
	cadeia := func(h http.Handler, ms ...mw) http.Handler {
		for i := len(ms) - 1; i >= 0; i-- {
			h = ms[i](h)
		}
		return h
	}
	maxLote := int64(d.Cfg.Protocolo.LoteMaxBlocos*1059 + 64)
	base := func(rota string, h http.HandlerFunc, ms ...mw) http.Handler {
		return cadeia(h, append([]mw{recuperar(d.Log), d.instrumentar(rota)}, ms...)...)
	}
	mux.Handle("POST /v1/dongles/{id}/desafio", base("desafio", d.pedirDesafio,
		d.limitarIP, d.limitarDevID("desafio", d.Cfg.Limites.DesafioMin)))
	mux.Handle("POST /v1/dongles/{id}/desafio/resposta", base("resposta", d.responderDesafio,
		anotarS2, d.limitarIP, d.limitarDevID("resposta", d.Cfg.Limites.RespostaMin)))
	mux.Handle("POST /v1/dongles/blocos", base("blocos", d.receberLote,
		d.limitarIP, d.autenticarJWT(false), d.limitarDevID("blocos", d.Cfg.Limites.BlocosMin), d.limitarConcorrencia, limitarCorpo(maxLote)))
	mux.Handle("GET /v1/dongles/{id}/confirmacao", base("confirmacao", d.obterConfirmacao,
		d.limitarIP, d.autenticarJWT(true), d.limitarDevID("confirmacao", 60)))
	mux.HandleFunc("GET /saude/viva", d.viva)
	mux.HandleFunc("GET /saude/pronta", d.pronta)
	return mux
}
