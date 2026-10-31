package prometheus

import (
	"net/http"
	"strconv"
	"time"

	"tccelta/backend/internal/port"
)

// Os métodos de M que têm o nome de uma série ficam em adaptadores, um por porta.

type handshakePort struct{ m *M }

func (p handshakePort) Handshake(r string)        { p.m.Handshakes.WithLabelValues(r).Inc() }
func (p handshakePort) Gap(c string)              { p.m.Gaps.WithLabelValues(c).Inc() }
func (p handshakePort) AnchorRTT(d time.Duration) { p.m.AnchorRTT.Observe(d.Seconds()) }

type ingestionPort struct{ m *M }

func (p ingestionPort) Blocks(r string, n int) {
	if n > 0 {
		p.m.Blocks.WithLabelValues(r).Add(float64(n))
	}
}
func (p ingestionPort) Batch(s string)                      { p.m.Batches.WithLabelValues(s).Inc() }
func (p ingestionPort) IngestionTxDuration(d time.Duration) { p.m.IngestionTx.Observe(d.Seconds()) }
func (p ingestionPort) BlockVerifyDuration(profile string, d time.Duration) {
	p.m.BlockVerify.WithLabelValues(profile).Observe(d.Seconds())
}
func (p ingestionPort) Rows(t string, n int) { p.m.Rows.WithLabelValues(t).Add(float64(n)) }
func (p ingestionPort) Gap(c string)         { p.m.Gaps.WithLabelValues(c).Inc() }

type httpPort struct{ m *M }

func (p httpPort) Request(route string, status int, d time.Duration) {
	p.m.HTTPReq.WithLabelValues(route, strconv.Itoa(status)).Inc()
	p.m.HTTPDur.WithLabelValues(route).Observe(d.Seconds())
}
func (p httpPort) Rejection(reason string) { p.m.Rejections.WithLabelValues(reason).Inc() }

type kmsPort struct{ m *M }

func (p kmsPort) KMSCall(op, res, phase string, d time.Duration) {
	p.m.KMSCalls.WithLabelValues(op, res, phase).Inc()
	p.m.KMSDur.WithLabelValues(op).Observe(d.Seconds())
}
func (p kmsPort) KMSBreaker(open bool) {
	if open {
		p.m.KMSBreaker.Set(1)
	} else {
		p.m.KMSBreaker.Set(0)
	}
}

// HandshakePort devolve o adaptador da porta de métricas do handshake.
func (m *M) HandshakePort() port.MetricsHandshake { return handshakePort{m} }

// IngestionPort devolve o adaptador da porta de métricas da ingestão.
func (m *M) IngestionPort() port.MetricsIngestion { return ingestionPort{m} }

// HTTPPort devolve o adaptador da porta de métricas HTTP.
func (m *M) HTTPPort() port.MetricsHTTP { return httpPort{m} }

// KMSPort devolve o adaptador da porta de métricas do KMS.
func (m *M) KMSPort() port.MetricsKMS { return kmsPort{m} }

// Server monta o servidor de /metricas no endereço dado (o chamador inicia e encerra).
func (m *M) Server(address string) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("GET /metricas", m.Handler())
	return &http.Server{Addr: address, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
}

type workerPort struct{ m *M }

func (p workerPort) Boot(source string)           { p.m.WorkerBoots.WithLabelValues(source).Inc() }
func (p workerPort) Alert(t string)               { p.m.WorkerAlerts.WithLabelValues(t).Inc() }
func (p workerPort) Groups(n int)                 { p.m.WorkerGroups.Add(float64(n)) }
func (p workerPort) BootDuration(d time.Duration) { p.m.WorkerDur.Observe(d.Seconds()) }
func (p workerPort) Expired(n int64)              { p.m.WorkerExpired.Add(float64(n)) }
func (p workerPort) Queue(n int64)                { p.m.WorkerQueue.Set(float64(n)) }
func (p workerPort) Reconnection()                { p.m.WorkerReconnect.Inc() }

type reprocessPort struct{ m *M }

func (p reprocessPort) Reprocessed(rows int) { p.m.Reprocessed.Add(float64(rows)) }

// WorkerPort devolve o adaptador da porta de métricas do worker.
func (m *M) WorkerPort() port.MetricsWorker { return workerPort{m} }

// ReprocessPort devolve o adaptador da porta de métricas do reprocessamento.
func (m *M) ReprocessPort() port.MetricsReprocess { return reprocessPort{m} }
