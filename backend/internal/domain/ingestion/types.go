package ingestion

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Erros do lote.
var (
	ErrMalformedBatch = errors.New("lote_malformado")
	ErrBatchTooLarge  = errors.New("lote_grande")
	ErrNoConfirmation = errors.New("sem_confirmacao")
	ErrInvariant      = errors.New("invariante_violada")
)

// RawBlock é uma linha de bronze.raw_block.
type RawBlock struct {
	Seq        int64
	BootID     int32
	Generation int32
	T0Ms       int64
	Marker     string
	Payload    []byte
	Signature  []byte
}

// Rejected é um bloco recusado, guardado para auditoria.
type Rejected struct {
	DongleID    int32
	Seq         int64
	BootID      int32
	T0Ms        int64
	Payload     []byte
	Signature   []byte
	Reason      Reason
	Detail      string
	ContentHash []byte
	TokenJTI    uuid.UUID
	ReceivedAt  time.Time
}

// BootAggregate resume os blocos aceitos de um (geração, boot_id) no lote.
type BootAggregate struct {
	Generation, BootID           int32
	FromSeq, ToSeq, TStart, TEnd int64
	MaxOffset                    float64
}

// StateUpdate é o novo estado do dongle depois de um lote.
type StateUpdate struct {
	DongleID                   int32
	ContiguousSeq, MaxSeen     int64
	LastAck                    []byte
	Now                        time.Time
	BootGeneration, LastBootID int32
}

// StoredBlock é um bloco de dados já gravado em bronze.raw_block, lido para o reprocessamento.
type StoredBlock struct {
	Seq        int64
	Generation int32
	BootID     int32
	T0Ms       int64
	Payload    []byte
	ReceivedAt time.Time
}
