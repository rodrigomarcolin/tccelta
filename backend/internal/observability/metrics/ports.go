package metricas

import (
	"net/http"
	"strconv"
	"time"

	"tccelta/backend/internal/casodeuso/porta"
)

// Os métodos de M que têm o nome de uma série ficam em adaptadores, um por porta.

type portaHandshake struct{ m *M }

func (p portaHandshake) Handshake(r string)        { p.m.Handshakes.WithLabelValues(r).Inc() }
func (p portaHandshake) Lacuna(c string)           { p.m.Lacunas.WithLabelValues(c).Inc() }
func (p portaHandshake) AncoraRTT(d time.Duration) { p.m.AncoraRTT.Observe(d.Seconds()) }

type portaIngestao struct{ m *M }

func (p portaIngestao) Blocos(r string, n int) {
	if n > 0 {
		p.m.Blocos.WithLabelValues(r).Add(float64(n))
	}
}
func (p portaIngestao) Lote(s string)                     { p.m.Lotes.WithLabelValues(s).Inc() }
func (p portaIngestao) DuracaoTxIngestao(d time.Duration) { p.m.IngestaoTx.Observe(d.Seconds()) }
func (p portaIngestao) Linhas(t string, n int)            { p.m.Linhas.WithLabelValues(t).Add(float64(n)) }
func (p portaIngestao) Lacuna(c string)                   { p.m.Lacunas.WithLabelValues(c).Inc() }

type portaHTTP struct{ m *M }

func (p portaHTTP) Requisicao(rota string, status int, d time.Duration) {
	p.m.HTTPReq.WithLabelValues(rota, strconv.Itoa(status)).Inc()
	p.m.HTTPDur.WithLabelValues(rota).Observe(d.Seconds())
}
func (p portaHTTP) Rejeicao(motivo string) { p.m.Rejeicoes.WithLabelValues(motivo).Inc() }

type portaKMS struct{ m *M }

func (p portaKMS) ChamadaKMS(op, res, fase string, d time.Duration) {
	p.m.KMSChamadas.WithLabelValues(op, res, fase).Inc()
	p.m.KMSDur.WithLabelValues(op).Observe(d.Seconds())
}
func (p portaKMS) DisjuntorKMS(aberto bool) {
	if aberto {
		p.m.KMSDisjuntor.Set(1)
	} else {
		p.m.KMSDisjuntor.Set(0)
	}
}

// PortaHandshake devolve o adaptador da porta de métricas do handshake.
func (m *M) PortaHandshake() porta.MetricasHandshake { return portaHandshake{m} }

// PortaIngestao devolve o adaptador da porta de métricas da ingestão.
func (m *M) PortaIngestao() porta.MetricasIngestao { return portaIngestao{m} }

// PortaHTTP devolve o adaptador da porta de métricas HTTP.
func (m *M) PortaHTTP() porta.MetricasHTTP { return portaHTTP{m} }

// PortaKMS devolve o adaptador da porta de métricas do KMS.
func (m *M) PortaKMS() porta.MetricasKMS { return portaKMS{m} }

// Servidor monta o servidor de /metricas no endereço dado (o chamador inicia e encerra).
func (m *M) Servidor(endereco string) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("GET /metricas", m.Handler())
	return &http.Server{Addr: endereco, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
}
