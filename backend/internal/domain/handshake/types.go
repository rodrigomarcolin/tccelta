package handshake

import (
	"time"

	"github.com/google/uuid"
)

// Resultados gravados em desafio.resultado.
const (
	ResultadoAceito          = "aceito"
	ResultadoAceitoSemAncora = "aceito_sem_ancora"
	ResultadoInconsistente   = "inconsistente"
)

// Emissao pede um novo desafio para o dongle.
type Emissao struct {
	DongleID     int32
	S1           time.Time
	ExpiraEm     time.Time
	Nonce        []byte
	MaxPendentes int
}

// Desafio é o desafio recém-emitido: c_S novo e a posição confirmada do dongle.
type Desafio struct {
	CS, SeqContiguo, CConf int64
}

// DesafioGravado é o desafio lido para a pré-checagem da resposta.
type DesafioGravado struct {
	Nonce        []byte
	S1, ExpiraEm time.Time
	RespondidoEm *time.Time
}

// Consumo marca o desafio como respondido e guarda o que o dongle relatou.
type Consumo struct {
	DongleID              int32
	CS                    int64
	Nonce                 []byte
	S2                    time.Time
	BootID                int32
	TMs, HeadSeq, TailSeq int64
}

// Finalizacao fecha o desafio com o resultado do handshake.
type Finalizacao struct {
	DongleID         int32
	CS               int64
	Resultado        string
	AckSeq           *int64
	PerdaIni         *int64
	PerdaFim         *int64
	TokenJTI         *uuid.UUID
	TMsDesembrulhado *int64
	GeracaoBoot      *int32
}

// Ancora liga o t_ms do dongle ao relógio do servidor.
type Ancora struct {
	DongleID    int32
	GeracaoBoot int32
	BootID      int32
	TMs         int64
	Nonce       []byte
	S1, S2      time.Time
}

// Atualizacao é o novo estado do dongle depois de um handshake aceito.
type Atualizacao struct {
	DongleID             int32
	SeqContiguo, HeadSeq int64
	TailSeq              int64
	S2, Agora            time.Time
	CConf                int64
	UltimoAck            []byte
}
