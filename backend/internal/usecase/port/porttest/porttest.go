// Package porttest tem dublês das portas compartilhadas, para testes e para o simulador.
package porttest

import (
	"time"

	"tccelta/backend/internal/usecase/port"
)

// Nop descarta todas as métricas; implementa todas as portas de métricas.
type Nop struct{}

func (Nop) Handshake(string)                              {}
func (Nop) Gap(string)                                    {}
func (Nop) AnchorRTT(time.Duration)                       {}
func (Nop) Blocks(string, int)                            {}
func (Nop) Batch(string)                                  {}
func (Nop) IngestionTxDuration(time.Duration)             {}
func (Nop) Rows(string, int)                              {}
func (Nop) Request(string, int, time.Duration)            {}
func (Nop) Rejection(string)                              {}
func (Nop) KMSCall(string, string, string, time.Duration) {}
func (Nop) KMSBreaker(bool)                               {}

var (
	_ port.MetricsHandshake = Nop{}
	_ port.MetricsIngestion = Nop{}
	_ port.MetricsHTTP      = Nop{}
	_ port.MetricsKMS       = Nop{}
)

// FixedClock devolve um relógio que sempre responde t. Para avançar, use AdjustableClock.
func FixedClock(t time.Time) port.Clock { return func() time.Time { return t } }

// AdjustableClock é um relógio controlado pelo teste.
type AdjustableClock struct{ T time.Time }

// Now implementa port.Clock via método (use r.Now como valor).
func (r *AdjustableClock) Now() time.Time { return r.T }

// Advance soma d ao relógio.
func (r *AdjustableClock) Advance(d time.Duration) { r.T = r.T.Add(d) }
