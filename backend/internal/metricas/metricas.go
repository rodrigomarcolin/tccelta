// Package metricas concentra os registradores Prometheus.
package metricas

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// M agrupa todas as séries. Use Nova().
type M struct {
	Reg *prometheus.Registry

	HTTPReq       *prometheus.CounterVec
	HTTPDur       *prometheus.HistogramVec
	Blocos        *prometheus.CounterVec
	Lotes         *prometheus.CounterVec
	IngestaoTx    prometheus.Histogram
	Linhas        *prometheus.CounterVec
	Handshakes    *prometheus.CounterVec
	AncoraRTT     prometheus.Histogram
	Lacunas       *prometheus.CounterVec
	KMSChamadas   *prometheus.CounterVec
	KMSDur        *prometheus.HistogramVec
	KMSDisjuntor  prometheus.Gauge
	Rejeicoes     *prometheus.CounterVec
	RedisFalhas   prometheus.Counter
	Reprocessadas prometheus.Counter

	WorkerBoots     *prometheus.CounterVec
	WorkerDur       prometheus.Histogram
	WorkerFila      prometheus.Gauge
	WorkerAlertas   *prometheus.CounterVec
	WorkerExpirados prometheus.Counter
	WorkerReconex   prometheus.Counter
	WorkerGrupos    prometheus.Counter
}

// Nova cria o conjunto de métricas em um registro próprio.
func Nova() *M {
	r := prometheus.NewRegistry()
	m := &M{Reg: r}
	m.HTTPReq = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "tccelta_http_requisicoes_total"}, []string{"rota", "status"})
	m.HTTPDur = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "tccelta_http_duracao_segundos",
		Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}}, []string{"rota"})
	m.Blocos = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "tccelta_blocos_total"}, []string{"resultado"})
	m.Lotes = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "tccelta_lotes_total"}, []string{"status"})
	m.IngestaoTx = prometheus.NewHistogram(prometheus.HistogramOpts{Name: "tccelta_ingestao_tx_duracao_segundos",
		Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5}})
	m.Linhas = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "tccelta_linhas_inseridas_total"}, []string{"tabela"})
	m.Handshakes = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "tccelta_handshakes_total"}, []string{"resultado"})
	m.AncoraRTT = prometheus.NewHistogram(prometheus.HistogramOpts{Name: "tccelta_ancora_rtt_segundos",
		Buckets: []float64{.05, .1, .25, .5, 1, 2, 5, 10}})
	m.Lacunas = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "tccelta_lacunas_total"}, []string{"causa"})
	m.KMSChamadas = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "tccelta_kms_chamadas_total"}, []string{"operacao", "resultado", "fase"})
	m.KMSDur = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "tccelta_kms_duracao_segundos",
		Buckets: []float64{.001, .0025, .005, .01, .025, .05, .1, .25, .5, 1, 2}}, []string{"operacao"})
	m.KMSDisjuntor = prometheus.NewGauge(prometheus.GaugeOpts{Name: "tccelta_kms_disjuntor_aberto"})
	m.Rejeicoes = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "tccelta_rejeicoes_total"}, []string{"motivo"})
	m.RedisFalhas = prometheus.NewCounter(prometheus.CounterOpts{Name: "tccelta_limite_redis_falhas_total"})
	m.Reprocessadas = prometheus.NewCounter(prometheus.CounterOpts{Name: "tccelta_reprocessamento_linhas_total"})
	m.WorkerBoots = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "tccelta_worker_boots_processados_total"}, []string{"origem"})
	m.WorkerDur = prometheus.NewHistogram(prometheus.HistogramOpts{Name: "tccelta_worker_duracao_segundos",
		Buckets: []float64{.001, .005, .01, .05, .1, .5, 1, 5}})
	m.WorkerFila = prometheus.NewGauge(prometheus.GaugeOpts{Name: "tccelta_worker_fila_tempo"})
	m.WorkerAlertas = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "tccelta_worker_alertas_total"}, []string{"tipo"})
	m.WorkerExpirados = prometheus.NewCounter(prometheus.CounterOpts{Name: "tccelta_worker_desafios_expirados_total"})
	m.WorkerReconex = prometheus.NewCounter(prometheus.CounterOpts{Name: "tccelta_worker_escuta_reconexoes_total"})
	m.WorkerGrupos = prometheus.NewCounter(prometheus.CounterOpts{Name: "tccelta_worker_grupos_gravados_total"})
	for _, c := range []prometheus.Collector{m.HTTPReq, m.HTTPDur, m.Blocos, m.Lotes, m.IngestaoTx, m.Linhas, m.Handshakes,
		m.AncoraRTT, m.Lacunas, m.KMSChamadas, m.KMSDur, m.KMSDisjuntor, m.Rejeicoes, m.RedisFalhas, m.Reprocessadas,
		m.WorkerBoots, m.WorkerDur, m.WorkerFila, m.WorkerAlertas, m.WorkerExpirados, m.WorkerReconex, m.WorkerGrupos} {
		r.MustRegister(c)
	}
	return m
}

// RegistrarPool expõe tccelta_pool_* a partir de pgxpool.Stat().
func (m *M) RegistrarPool(p *pgxpool.Pool) {
	g := func(nome string, f func() float64) {
		m.Reg.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "tccelta_pool_" + nome}, f))
	}
	g("conexoes_total", func() float64 { return float64(p.Stat().TotalConns()) })
	g("conexoes_em_uso", func() float64 { return float64(p.Stat().AcquiredConns()) })
	g("conexoes_ociosas", func() float64 { return float64(p.Stat().IdleConns()) })
}

// Handler serve /metricas.
func (m *M) Handler() http.Handler {
	return promhttp.HandlerFor(m.Reg, promhttp.HandlerOpts{})
}
