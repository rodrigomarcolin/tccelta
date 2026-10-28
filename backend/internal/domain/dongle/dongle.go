// Package dongle é o cadastro do dongle e a regra de acesso.
package dongle

import (
	"errors"

	"tccelta/backend/internal/dominio/protocolo"
)

// Erros de acesso.
var (
	ErrDesconhecido       = errors.New("dongle_desconhecido")
	ErrInativo            = errors.New("dongle_inativo")
	ErrPerfilNaoPermitido = errors.New("perfil_nao_permitido")
)

// Dongle é a linha de cadastro usada pelo caminho quente.
type Dongle struct {
	ID           int32
	DevID        protocolo.DevID
	Perfil       protocolo.Perfil
	ChaveAuth    string
	ChaveRec     string
	ChavePublica []byte
	Ativo        bool
}

// VerificarAcesso: dongle precisa estar ativo; o perfil "none" só passa com permitirNone.
func (d Dongle) VerificarAcesso(permitirNone bool) error {
	if !d.Ativo {
		return ErrInativo
	}
	if d.Perfil.Nome == "none" && !permitirNone {
		return ErrPerfilNaoPermitido
	}
	return nil
}

// Estado é o recorte de dongle_estado usado pelo handshake e pela ingestão.
type Estado struct {
	SeqContiguo, MaiorSeqVisto, HeadSeq, CConf int64
	GeracaoBoot, UltimoBootID                  int32
	// UltimoAck só é preenchido por LerSnapshot.
	UltimoAck []byte
}
