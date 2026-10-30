// Package occurrence define lacunas e alertas, os registros de anomalia do dongle.
package occurrence

import "time"

// Causas de lacuna.
const (
	CauseOverflow = "OVERFLOW"
	CauseBoot     = "BOOT"
	CauseLoss     = "LOSS"
)

// Tipos de alerta.
const (
	AlertServerClockRegressed   = "server_clock_regressed"
	AlertUnknownPID             = "unknown_pid"
	AlertInvariantViolated      = "invariant_violated"
	AlertBootGenerationAdvanced = "boot_generation_advanced"
)

// Gap é um intervalo de seq perdido. CS (handshake) e BlockSeq (ingestão) são opcionais.
type Gap struct {
	DongleID     int32
	From, To     int64
	Cause        string
	RegisteredAt time.Time
	CS           *int64
	BlockSeq     *int64
}

// Alert é um evento a investigar. Detail é JSON.
type Alert struct {
	DongleID       int32
	BootGeneration *int32
	BootID         *int32
	Type           string
	Detail         []byte
}
