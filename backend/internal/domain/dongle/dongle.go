// Package dongle é o cadastro do dongle e a regra de acesso.
package dongle

import (
	"errors"
	"time"

	"tccelta/backend/internal/domain/protocol"
)

// Erros de acesso.
var (
	ErrUnknown           = errors.New("dongle_desconhecido")
	ErrInactive          = errors.New("dongle_inativo")
	ErrProfileNotAllowed = errors.New("perfil_nao_permitido")
)

// Dongle é a linha de cadastro usada pelo caminho quente.
type Dongle struct {
	ID        int32
	DevID     protocol.DevID
	Profile   protocol.Profile
	AuthKey   string
	RecKey    string
	PublicKey []byte
	Active    bool
}

// VerifyAccess: dongle precisa estar ativo; o perfil "none" só passa com allowNone.
func (d Dongle) VerifyAccess(allowNone bool) error {
	if !d.Active {
		return ErrInactive
	}
	if d.Profile.Name == "none" && !allowNone {
		return ErrProfileNotAllowed
	}
	return nil
}

// State é o recorte de dongle_state usado pelo handshake e pela ingestão.
type State struct {
	ContiguousSeq, MaxSeenSeq, HeadSeq, CConf int64
	BootGeneration, LastBootID                int32
	// LastAck só é preenchido por ReadSnapshot.
	LastAck []byte
}

// Registration é o cadastro de um dongle provisionado. Profile é hmac, asym ou none; PublicKey só existe no asym.
type Registration struct {
	DevID       protocol.DevID
	Profile     string
	AuthKey     string
	RecKey      string
	PublicKey   []byte
	Description string
}

// Progress é o avanço da ingestão do dongle, lido pelo worker de viagens (dongle_state).
type Progress struct {
	ContiguousSeq, HeadSeq int64
	// LastAckAt é nulo se nenhum lote ou handshake foi confirmado ainda.
	LastAckAt *time.Time
}
