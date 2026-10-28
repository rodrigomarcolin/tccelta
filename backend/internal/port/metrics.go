package port

import "time"

// MetricsHandshake são os eventos do handshake.
type MetricsHandshake interface {
	Handshake(result string)
	Gap(cause string)
	AnchorRTT(d time.Duration)
}

// MetricsIngestion são os eventos do lote.
type MetricsIngestion interface {
	Blocks(result string, n int)
	Batch(status string)
	IngestionTxDuration(d time.Duration)
	Rows(table string, n int)
	Gap(cause string)
}

// MetricsHTTP são os eventos de transporte.
type MetricsHTTP interface {
	Request(route string, status int, d time.Duration)
	Rejection(reason string)
}

// MetricsKMS são os eventos das chamadas ao KMS.
type MetricsKMS interface {
	KMSCall(operation, result, phase string, d time.Duration)
	KMSBreaker(open bool)
}
