// Package dongle é o cadastro do dongle e a regra de acesso.
package dongle

import (
	"errors"

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
