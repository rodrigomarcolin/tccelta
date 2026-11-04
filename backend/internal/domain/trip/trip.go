// Package trip tem os tipos da viagem (gold.trip) e dos dados que o cálculo da análise carrega da silver. É puro: a
// persistência está nas portas (internal/port) e o cálculo, em domain/tripanalysis.
package trip

import (
	"time"

	"github.com/google/uuid"

	"tccelta/backend/internal/domain/tripanalysis"
)

// Status da viagem (gold.trip.status).
const (
	StatusPending     = "pending"
	StatusWaitingData = "waiting_data"
	StatusReady       = "ready"
	StatusFailed      = "failed"
)

// Trip é uma linha de gold.trip, com o que a fila e o cálculo usam.
type Trip struct {
	ID                   uuid.UUID
	Category, DriverName string
	Fuel                 string
	DongleID             int32
	EntryType            string // boot | interval
	BootGeneration       *int32
	BootID               *int32
	IntervalStart        *time.Time
	IntervalEnd          *time.Time
	Status               string
	TargetSeq            int64
	FirstSeq, LastSeq    *int64
	CalculatedThroughSeq *int64
	Provisional          bool
	RecalcPending        bool
	CalculationVersion   *string
	CreatedAt            time.Time
	CalculatedAt         *time.Time
}

// Entry converte a viagem na entrada da análise.
func (t Trip) Entry() tripanalysis.Entry {
	e := tripanalysis.Entry{Type: tripanalysis.EntryBoot, DongleID: t.DongleID, BootGeneration: t.BootGeneration, BootID: t.BootID}
	if t.EntryType == "interval" {
		e = tripanalysis.Entry{Type: tripanalysis.EntryInterval, DongleID: t.DongleID, T0: t.IntervalStart, T1: t.IntervalEnd}
	}
	return e
}

// BootRef identifica um boot.
type BootRef struct{ Generation, BootID int32 }

// Provisional é uma viagem pronta mas provisória, com os boots que ficaram de fora por falta de hora (boot_sem_hora).
type Provisional struct {
	Trip
	NoTimeBoots []BootRef
}

// Boot é a linha de boot com a faixa e a reta de tempo.
type Boot struct {
	Generation, BootID int32
	FirstSeq, LastSeq  *int64
	FirstTMs, LastTMs  *int64
	MaxOffsetMs        *float64
	AMs, B             *float64
	BaseUncertaintyMs  *int32
	TimeSource         *string
}

// Ref devolve a chave do boot.
func (b Boot) Ref() BootRef { return BootRef{b.Generation, b.BootID} }

// HasTime diz se o boot tem conversão para hora absoluta.
func (b Boot) HasTime() bool { return b.TimeSource != nil && b.AMs != nil && b.B != nil }

// EpochMs converte um t_ms do boot em hora absoluta (ms desde a época Unix). Só vale com HasTime.
func (b Boot) EpochMs(tMs float64) float64 { return *b.AMs + (1+*b.B)*tMs }

// ReadingQuery seleciona as leituras de um boot. O intervalo de received_at é obrigatório (a hypertable é particionada por ele).
type ReadingQuery struct {
	DongleID                 int32
	Generation, BootID       int32
	PIDs                     []int16
	ReceivedFrom, ReceivedTo time.Time
	TFrom, TTo               int64
}

// ReadingRow é uma leitura decodificada (value não nulo).
type ReadingRow struct {
	PID   int16
	TMs   int64
	Value float64
}

// PositionRow é um ponto do GPS.
type PositionRow struct {
	TMs      int64
	Lat, Lon float64
}

// Calculation é o que Save grava: o resultado da análise e o estado da viagem.
type Calculation struct {
	Result               tripanalysis.Result
	FirstSeq, LastSeq    *int64
	CalculatedThroughSeq int64
	Provisional          bool
	CalculatedAt         time.Time
}

// Queue são os tamanhos das filas de viagens.
type Queue struct{ Waiting, Recalc int64 }
