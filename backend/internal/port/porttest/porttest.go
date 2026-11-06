// Package porttest tem dublês das portas compartilhadas, para testes, para o simulador e para comandos sem métricas.
package porttest

import (
	"time"

	"tccelta/backend/internal/port"
)

// Nop descarta todas as métricas; implementa todas as portas de métricas.
type Nop struct{}

func (Nop) Handshake(string)                              {}
func (Nop) Gap(string)                                    {}
func (Nop) AnchorRTT(time.Duration)                       {}
func (Nop) Blocks(string, int)                            {}
func (Nop) Batch(string)                                  {}
func (Nop) IngestionTxDuration(time.Duration)             {}
func (Nop) BlockVerifyDuration(string, time.Duration)     {}
func (Nop) Rows(string, int)                              {}
func (Nop) Request(string, int, time.Duration)            {}
func (Nop) Rejection(string)                              {}
func (Nop) KMSCall(string, string, string, time.Duration) {}
func (Nop) KMSBreaker(bool)                               {}
func (Nop) Boot(string)                                   {}
func (Nop) Alert(string)                                  {}
func (Nop) Groups(int)                                    {}
func (Nop) BootDuration(time.Duration)                    {}
func (Nop) Expired(int64)                                 {}
func (Nop) Queue(int64)                                   {}
func (Nop) Reconnection()                                 {}
func (Nop) Trip(string)                                   {}
func (Nop) TripPartial()                                  {}
func (Nop) TripProvisional(string)                        {}
func (Nop) TripDuration(time.Duration)                    {}
func (Nop) TripWait(time.Duration)                        {}
func (Nop) TripQueue(int64, int64)                        {}
func (Nop) Reprocessed(int)                               {}
func (Nop) TokenRequest(string)                           {}
func (Nop) Block(string)                                  {}
func (Nop) BlockStoreFailure()                            {}

var (
	_ port.MetricsHandshake = Nop{}
	_ port.MetricsIngestion = Nop{}
	_ port.MetricsHTTP      = Nop{}
	_ port.MetricsKMS       = Nop{}
	_ port.MetricsWorker    = Nop{}
	_ port.MetricsReprocess = Nop{}
	_ port.MetricsAuth      = Nop{}
)

// FixedClock devolve um relógio que sempre responde t. Para avançar, use AdjustableClock.
func FixedClock(t time.Time) port.Clock { return func() time.Time { return t } }

// AdjustableClock é um relógio controlado pelo teste.
type AdjustableClock struct{ T time.Time }

// Now implementa port.Clock via método (use r.Now como valor).
func (r *AdjustableClock) Now() time.Time { return r.T }

// Advance soma d ao relógio.
func (r *AdjustableClock) Advance(d time.Duration) { r.T = r.T.Add(d) }
