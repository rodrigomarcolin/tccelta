// Package metricas tem os registradores Prometheus do serviço KMS (pasta própria, como na API).
// As séries levam o prefixo tccelta_kmssvc_ para não colidir com as tccelta_kms_* que a API conta
// do lado do cliente.
package metricas

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// M agrupa as séries. Use Nova().
type M struct {
	Reg *prometheus.Registry

	HTTPReq      *prometheus.CounterVec
	HTTPDur      *prometheus.HistogramVec
	VaultCalls   *prometheus.CounterVec
	VaultDur     *prometheus.HistogramVec
	VaultDisj    prometheus.Gauge
	Rejeicoes    *prometheus.CounterVec
	JWTRotacoes  prometheus.Counter
	JWTRotacaoOk prometheus.Gauge
}

// Nova cria o conjunto de métricas em um registro próprio.
func Nova() *M {
	r := prometheus.NewRegistry()
	m := &M{Reg: r}
	m.HTTPReq = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "tccelta_kmssvc_http_requisicoes_total"}, []string{"rota", "status"})
	m.HTTPDur = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "tccelta_kmssvc_http_duracao_segundos",
		Buckets: []float64{.001, .0025, .005, .01, .025, .05, .1, .25, .5, 1, 2}}, []string{"rota"})
	m.VaultCalls = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "tccelta_kmssvc_vault_chamadas_total"}, []string{"operacao", "resultado"})
	m.VaultDur = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "tccelta_kmssvc_vault_duracao_segundos",
		Buckets: []float64{.001, .0025, .005, .01, .025, .05, .1, .25, .5, 1, 2}}, []string{"operacao"})
	m.VaultDisj = prometheus.NewGauge(prometheus.GaugeOpts{Name: "tccelta_kmssvc_vault_disjuntor_aberto"})
	m.Rejeicoes = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "tccelta_kmssvc_rejeicoes_total"}, []string{"motivo"})
	m.JWTRotacoes = prometheus.NewCounter(prometheus.CounterOpts{Name: "tccelta_kmssvc_jwt_rotacoes_total"})
	m.JWTRotacaoOk = prometheus.NewGauge(prometheus.GaugeOpts{Name: "tccelta_kmssvc_jwt_ultima_verificacao_ok"})
	for _, c := range []prometheus.Collector{m.HTTPReq, m.HTTPDur, m.VaultCalls, m.VaultDur, m.VaultDisj, m.Rejeicoes, m.JWTRotacoes, m.JWTRotacaoOk} {
		r.MustRegister(c)
	}
	return m
}

// ChamadaVault implementa vault.Observador.
func (m *M) ChamadaVault(op, res string, d time.Duration) {
	m.VaultCalls.WithLabelValues(op, res).Inc()
	m.VaultDur.WithLabelValues(op).Observe(d.Seconds())
}

// Disjuntor implementa vault.Observador.
func (m *M) Disjuntor(aberto bool) {
	if aberto {
		m.VaultDisj.Set(1)
	} else {
		m.VaultDisj.Set(0)
	}
}

// Requisicao conta uma requisição HTTP atendida.
func (m *M) Requisicao(rota string, status int, d time.Duration) {
	m.HTTPReq.WithLabelValues(rota, strconv.Itoa(status)).Inc()
	m.HTTPDur.WithLabelValues(rota).Observe(d.Seconds())
}

// Rejeicao conta uma requisição recusada antes de chegar ao Vault.
func (m *M) Rejeicao(motivo string) { m.Rejeicoes.WithLabelValues(motivo).Inc() }

// Servidor monta o servidor de /metricas (porta separada da API do KMS).
func (m *M) Servidor(endereco string) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("GET /metricas", promhttp.HandlerFor(m.Reg, promhttp.HandlerOpts{}))
	return &http.Server{Addr: endereco, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
}
