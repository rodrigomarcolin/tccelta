package porta

import "time"

// MetricasHandshake são os eventos do handshake.
type MetricasHandshake interface {
	Handshake(resultado string)
	Lacuna(causa string)
	AncoraRTT(d time.Duration)
}

// MetricasIngestao são os eventos do lote.
type MetricasIngestao interface {
	Blocos(resultado string, n int)
	Lote(status string)
	DuracaoTxIngestao(d time.Duration)
	Linhas(tabela string, n int)
	Lacuna(causa string)
}

// MetricasHTTP são os eventos de transporte.
type MetricasHTTP interface {
	Requisicao(rota string, status int, d time.Duration)
	Rejeicao(motivo string)
}

// MetricasKMS são os eventos das chamadas ao KMS.
type MetricasKMS interface {
	ChamadaKMS(operacao, resultado, fase string, d time.Duration)
	DisjuntorKMS(aberto bool)
}
