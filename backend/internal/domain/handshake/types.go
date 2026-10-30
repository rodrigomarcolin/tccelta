package handshake

import (
	"time"

	"github.com/google/uuid"
)

// Resultados gravados em challenge.result.
const (
	ResultAccepted         = "accepted"
	ResultAcceptedNoAnchor = "accepted_no_anchor"
	ResultInconsistent     = "inconsistent"
)

// Issuance pede um novo desafio para o dongle.
type Issuance struct {
	DongleID   int32
	S1         time.Time
	ExpiresAt  time.Time
	Nonce      []byte
	MaxPending int
}

// Challenge é o desafio recém-emitido: c_S novo e a posição confirmada do dongle.
type Challenge struct {
	CS, ContiguousSeq, CConf int64
}

// StoredChallenge é o desafio lido para a pré-checagem da resposta.
type StoredChallenge struct {
	Nonce         []byte
	S1, ExpiresAt time.Time
	AnsweredAt    *time.Time
}

// Consumption marca o desafio como respondido e guarda o que o dongle relatou.
type Consumption struct {
	DongleID              int32
	CS                    int64
	Nonce                 []byte
	S2                    time.Time
	BootID                int32
	TMs, HeadSeq, TailSeq int64
}

// Finalization fecha o desafio com o resultado do handshake.
type Finalization struct {
	DongleID       int32
	CS             int64
	Result         string
	AckSeq         *int64
	LossStart      *int64
	LossEnd        *int64
	TokenJTI       *uuid.UUID
	UnwrappedTMs   *int64
	BootGeneration *int32
}

// Anchor liga o t_ms do dongle ao relógio do servidor.
type Anchor struct {
	DongleID       int32
	BootGeneration int32
	BootID         int32
	TMs            int64
	Nonce          []byte
	S1, S2         time.Time
}

// Update é o novo estado do dongle depois de um handshake aceito.
type Update struct {
	DongleID               int32
	ContiguousSeq, HeadSeq int64
	TailSeq                int64
	S2, Now                time.Time
	CConf                  int64
	LastAck                []byte
}
