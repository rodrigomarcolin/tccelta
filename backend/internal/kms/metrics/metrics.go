// Package metrics tem os registradores Prometheus do serviço KMS (pasta própria, como na API).
// As séries levam o prefixo tccelta_kmssvc_ para não colidir com as tccelta_kms_* que a API conta
// do lado do cliente.
package metrics

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

	HTTPReq       *prometheus.CounterVec
	HTTPDur       *prometheus.HistogramVec
	VaultCalls    *prometheus.CounterVec
	VaultDur      *prometheus.HistogramVec
	VaultBreaker  prometheus.Gauge
	Rejections    *prometheus.CounterVec
	JWTRotations  prometheus.Counter
	JWTRotationOK prometheus.Gauge
}

// New cria o conjunto de métricas em um registro próprio.
func New() *M {
	r := prometheus.NewRegistry()
	m := &M{Reg: r}
	m.HTTPReq = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "tccelta_kmssvc_http_requests_total"}, []string{"route", "status"})
	m.HTTPDur = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "tccelta_kmssvc_http_duration_seconds",
		Buckets: []float64{.001, .0025, .005, .01, .025, .05, .1, .25, .5, 1, 2}}, []string{"route"})
	m.VaultCalls = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "tccelta_kmssvc_vault_calls_total"}, []string{"operation", "result"})
	m.VaultDur = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "tccelta_kmssvc_vault_duration_seconds",
		Buckets: []float64{.001, .0025, .005, .01, .025, .05, .1, .25, .5, 1, 2}}, []string{"operation"})
	m.VaultBreaker = prometheus.NewGauge(prometheus.GaugeOpts{Name: "tccelta_kmssvc_vault_breaker_open"})
	m.Rejections = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "tccelta_kmssvc_rejections_total"}, []string{"reason"})
	m.JWTRotations = prometheus.NewCounter(prometheus.CounterOpts{Name: "tccelta_kmssvc_jwt_rotations_total"})
	m.JWTRotationOK = prometheus.NewGauge(prometheus.GaugeOpts{Name: "tccelta_kmssvc_jwt_last_check_ok"})
	for _, c := range []prometheus.Collector{m.HTTPReq, m.HTTPDur, m.VaultCalls, m.VaultDur, m.VaultBreaker, m.Rejections, m.JWTRotations, m.JWTRotationOK} {
		r.MustRegister(c)
	}
	return m
}

// VaultCall implementa vault.Observer.
func (m *M) VaultCall(op, res string, d time.Duration) {
	m.VaultCalls.WithLabelValues(op, res).Inc()
	m.VaultDur.WithLabelValues(op).Observe(d.Seconds())
}

// Breaker implementa vault.Observer.
func (m *M) Breaker(open bool) {
	if open {
		m.VaultBreaker.Set(1)
	} else {
		m.VaultBreaker.Set(0)
	}
}

// HTTPRequest conta uma requisição HTTP atendida.
func (m *M) HTTPRequest(route string, status int, d time.Duration) {
	m.HTTPReq.WithLabelValues(route, strconv.Itoa(status)).Inc()
	m.HTTPDur.WithLabelValues(route).Observe(d.Seconds())
}

// Reject conta uma requisição recusada antes de chegar ao Vault.
func (m *M) Reject(reason string) { m.Rejections.WithLabelValues(reason).Inc() }

// Server monta o servidor de /metricas (porta separada da API do KMS).
func (m *M) Server(address string) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("GET /metricas", promhttp.HandlerFor(m.Reg, promhttp.HandlerOpts{}))
	return &http.Server{Addr: address, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
}
