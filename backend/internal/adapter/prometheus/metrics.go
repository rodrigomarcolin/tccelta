// Package prometheus concentra os registradores Prometheus e implementa as portas port.Metrics*.
package prometheus

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// M agrupa todas as séries. Use New().
type M struct {
	Reg *prometheus.Registry

	HTTPReq       *prometheus.CounterVec
	HTTPDur       *prometheus.HistogramVec
	Blocks        *prometheus.CounterVec
	Batches       *prometheus.CounterVec
	IngestionTx   prometheus.Histogram
	BlockVerify   *prometheus.HistogramVec
	Rows          *prometheus.CounterVec
	Handshakes    *prometheus.CounterVec
	AnchorRTT     prometheus.Histogram
	Gaps          *prometheus.CounterVec
	KMSCalls      *prometheus.CounterVec
	KMSDur        *prometheus.HistogramVec
	KMSBreaker    prometheus.Gauge
	Rejections    *prometheus.CounterVec
	RedisFailures prometheus.Counter
	Reprocessed   prometheus.Counter

	WorkerBoots     *prometheus.CounterVec
	WorkerDur       prometheus.Histogram
	WorkerQueue     prometheus.Gauge
	WorkerAlerts    *prometheus.CounterVec
	WorkerExpired   prometheus.Counter
	WorkerReconnect prometheus.Counter
	WorkerGroups    prometheus.Counter
}

// New cria o conjunto de métricas em um registro próprio.
func New() *M {
	r := prometheus.NewRegistry()
	m := &M{Reg: r}
	m.HTTPReq = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "tccelta_http_requests_total"}, []string{"route", "status"})
	m.HTTPDur = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "tccelta_http_duration_seconds",
		Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}}, []string{"route"})
	m.Blocks = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "tccelta_blocks_total"}, []string{"result"})
	m.Batches = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "tccelta_batches_total"}, []string{"status"})
	m.IngestionTx = prometheus.NewHistogram(prometheus.HistogramOpts{Name: "tccelta_ingestion_tx_duration_seconds",
		Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5}})
	m.BlockVerify = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "tccelta_block_verify_duration_seconds",
		Buckets: []float64{.0001, .0005, .001, .0025, .005, .01, .025, .05, .1, .25, .5, 1}}, []string{"profile"})
	m.Rows = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "tccelta_inserted_rows_total"}, []string{"table"})
	m.Handshakes = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "tccelta_handshakes_total"}, []string{"result"})
	m.AnchorRTT = prometheus.NewHistogram(prometheus.HistogramOpts{Name: "tccelta_anchor_rtt_seconds",
		Buckets: []float64{.05, .1, .25, .5, 1, 2, 5, 10}})
	m.Gaps = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "tccelta_gaps_total"}, []string{"cause"})
	m.KMSCalls = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "tccelta_kms_calls_total"}, []string{"operation", "result", "phase"})
	m.KMSDur = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "tccelta_kms_duration_seconds",
		Buckets: []float64{.001, .0025, .005, .01, .025, .05, .1, .25, .5, 1, 2}}, []string{"operation"})
	m.KMSBreaker = prometheus.NewGauge(prometheus.GaugeOpts{Name: "tccelta_kms_breaker_open"})
	m.Rejections = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "tccelta_rejections_total"}, []string{"reason"})
	m.RedisFailures = prometheus.NewCounter(prometheus.CounterOpts{Name: "tccelta_limiter_redis_failures_total"})
	m.Reprocessed = prometheus.NewCounter(prometheus.CounterOpts{Name: "tccelta_reprocessing_rows_total"})
	m.WorkerBoots = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "tccelta_worker_processed_boots_total"}, []string{"source"})
	m.WorkerDur = prometheus.NewHistogram(prometheus.HistogramOpts{Name: "tccelta_worker_duration_seconds",
		Buckets: []float64{.001, .005, .01, .05, .1, .5, 1, 5}})
	m.WorkerQueue = prometheus.NewGauge(prometheus.GaugeOpts{Name: "tccelta_worker_time_queue"})
	m.WorkerAlerts = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "tccelta_worker_alerts_total"}, []string{"type"})
	m.WorkerExpired = prometheus.NewCounter(prometheus.CounterOpts{Name: "tccelta_worker_expired_challenges_total"})
	m.WorkerReconnect = prometheus.NewCounter(prometheus.CounterOpts{Name: "tccelta_worker_listen_reconnections_total"})
	m.WorkerGroups = prometheus.NewCounter(prometheus.CounterOpts{Name: "tccelta_worker_anchor_groups_written_total"})
	for _, c := range []prometheus.Collector{m.HTTPReq, m.HTTPDur, m.Blocks, m.Batches, m.IngestionTx, m.BlockVerify, m.Rows, m.Handshakes,
		m.AnchorRTT, m.Gaps, m.KMSCalls, m.KMSDur, m.KMSBreaker, m.Rejections, m.RedisFailures, m.Reprocessed,
		m.WorkerBoots, m.WorkerDur, m.WorkerQueue, m.WorkerAlerts, m.WorkerExpired, m.WorkerReconnect, m.WorkerGroups} {
		r.MustRegister(c)
	}
	return m
}

// PoolStats são as contagens de conexões do pool de banco.
type PoolStats struct{ Total, InUse, Idle int32 }

// RegisterPool expõe tccelta_pool_* a partir de stats, que cmd liga ao pool (esse pacote não conhece o driver).
func (m *M) RegisterPool(stats func() PoolStats) {
	g := func(name string, f func() float64) {
		m.Reg.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "tccelta_pool_" + name}, f))
	}
	g("connections_total", func() float64 { return float64(stats().Total) })
	g("connections_in_use", func() float64 { return float64(stats().InUse) })
	g("connections_idle", func() float64 { return float64(stats().Idle) })
}

// Handler serve /metricas.
func (m *M) Handler() http.Handler {
	return promhttp.HandlerFor(m.Reg, promhttp.HandlerOpts{})
}
