package ingestao

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Erros do lote.
var (
	ErrLoteMalformado = errors.New("lote_malformado")
	ErrLoteGrande     = errors.New("lote_grande")
	ErrSemConfirmacao = errors.New("sem_confirmacao")
	ErrInvariante     = errors.New("invariante_violada")
)

// BlocoBruto é uma linha de bronze.bloco_bruto.
type BlocoBruto struct {
	Seq        int64
	BootID     int32
	Geracao    int32
	T0Ms       int64
	Marcador   string
	Payload    []byte
	Assinatura []byte
}

// Rejeitado é um bloco recusado, guardado para auditoria.
type Rejeitado struct {
	DongleID     int32
	Seq          int64
	BootID       int32
	T0Ms         int64
	Payload      []byte
	Assinatura   []byte
	Motivo       Motivo
	Detalhe      string
	HashConteudo []byte
	TokenJTI     uuid.UUID
	RecebidoEm   time.Time
}

// AgregadoBoot resume os blocos aceitos de um (geração, boot_id) no lote.
type AgregadoBoot struct {
	Geracao, BootID            int32
	SeqIni, SeqFim, TIni, TFim int64
	DeslMax                    float64
}

// AtualizacaoEstado é o novo estado do dongle depois de um lote.
type AtualizacaoEstado struct {
	DongleID                  int32
	SeqContiguo, MaiorVisto   int64
	UltimoAck                 []byte
	Agora                     time.Time
	GeracaoBoot, UltimoBootID int32
}
