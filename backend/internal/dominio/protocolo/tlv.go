// Package protocolo codifica e decodifica, byte a byte, as mensagens 0x01-0x04,
// os registros do bloco e a entrada do HMAC. Só usa a stdlib.
//
// Todos os inteiros são little-endian, exceto os 2 bytes de valor do registro
// de PID (A, B crus da resposta OBD-II).
package protocolo

import (
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// Rotulo é o prefixo da entrada do HMAC.
const Rotulo = "MALCONG"

// Tipo é o byte T do envelope TLV.
type Tipo uint8

const (
	TipoDesafio  Tipo = 0x01
	TipoResposta Tipo = 0x02
	TipoBloco    Tipo = 0x03
	TipoAck      Tipo = 0x04
)

// Tamanhos fixos.
const (
	TamTagAuth       = 16
	TamTagRec        = 32
	TamNonce         = 16
	TamCabBloco      = 12
	TamMinDados      = 17 // 12 de cabeçalho + 1 registro de PID (5)
	TamMaxDados      = 1024
	TamMarcador      = 11
	TamBlocoMarcador = TamCabBloco + TamMarcador
	LDesafio         = 45
	LDesafioDiag     = 47
	LResposta        = 50
	LAck             = 25
)

// DevID é o identificador de 8 bytes do dongle.
type DevID [8]byte

// DevIDDeHex converte exatamente 16 dígitos hexadecimais.
func DevIDDeHex(s string) (DevID, error) {
	var d DevID
	if len(s) != 16 {
		return d, ErrDevIDInvalido
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return d, ErrDevIDInvalido
	}
	copy(d[:], b)
	return d, nil
}

// Hex devolve o DevId em minúsculas.
func (d DevID) Hex() string { return hex.EncodeToString(d[:]) }

// TLV é um envelope T | L(u16 LE) | V. V aponta para o buffer original.
type TLV struct {
	T Tipo
	V []byte
}

// LerTLV lê um TLV do início de b e devolve os bytes consumidos.
func LerTLV(b []byte) (TLV, int, error) {
	if len(b) < 3 {
		return TLV{}, 0, ErrTLVCurto
	}
	l := int(binary.LittleEndian.Uint16(b[1:3]))
	if len(b) < 3+l {
		return TLV{}, 0, ErrTLVComprimento
	}
	return TLV{T: Tipo(b[0]), V: b[3 : 3+l : 3+l]}, 3 + l, nil
}

// LerLote lê a concatenação de TLVs; ErrLoteGrande se houver mais de max.
func LerLote(corpo []byte, max int) ([]TLV, error) {
	if len(corpo) == 0 {
		return nil, ErrLoteVazio
	}
	var out []TLV
	for len(corpo) > 0 {
		t, n, err := LerTLV(corpo)
		if err != nil {
			return nil, err
		}
		if len(out) >= max {
			return nil, ErrLoteGrande
		}
		out = append(out, t)
		corpo = corpo[n:]
	}
	return out, nil
}

// AnexarTLV acrescenta um TLV a dst.
func AnexarTLV(dst []byte, t Tipo, v []byte) []byte {
	dst = append(dst, byte(t), byte(len(v)), byte(len(v)>>8))
	return append(dst, v...)
}

// EntradaTag monta "MALCONG" || tipo || DevId || payload.
func EntradaTag(t Tipo, dev DevID, payload []byte) []byte {
	e := make([]byte, 0, len(Rotulo)+1+8+len(payload))
	e = append(e, Rotulo...)
	e = append(e, byte(t))
	e = append(e, dev[:]...)
	return append(e, payload...)
}

// CompararTag compara em tempo constante; tamanhos diferentes = false.
func CompararTag(calculada, recebida []byte) bool {
	if len(calculada) != len(recebida) {
		return false
	}
	return subtle.ConstantTimeCompare(calculada, recebida) == 1
}

// Perfil define só o tamanho da tag do BLOCO; a do handshake é sempre 16 B.
type Perfil struct {
	Nome        string
	TamTagBloco int
}

var (
	PerfilHMAC = Perfil{"hmac", 32}
	PerfilAsym = Perfil{"asym", 64}
	PerfilNone = Perfil{"none", 0}
)

// PerfilPorNome devolve o perfil pelo nome do cadastro.
func PerfilPorNome(n string) (Perfil, error) {
	switch strings.ToLower(n) {
	case "hmac":
		return PerfilHMAC, nil
	case "asym":
		return PerfilAsym, nil
	case "none":
		return PerfilNone, nil
	}
	return Perfil{}, fmt.Errorf("perfil desconhecido %q", n)
}

// Erros exportados.
var (
	ErrTLVCurto           = errors.New("protocolo: TLV curto")
	ErrTLVComprimento     = errors.New("protocolo: comprimento do TLV inconsistente")
	ErrTipoInesperado     = errors.New("protocolo: tipo inesperado")
	ErrLoteVazio          = errors.New("protocolo: lote vazio")
	ErrLoteGrande         = errors.New("protocolo: lote com TLVs demais")
	ErrTamanhoInvalido    = errors.New("protocolo: tamanho inválido")
	ErrRegistroMalformado = errors.New("protocolo: registro malformado")
	ErrMarcadorComOutros  = errors.New("protocolo: marcador junto com outros registros")
	ErrFlagsReservadas    = errors.New("protocolo: flags reservadas")
	ErrDevIDInvalido      = errors.New("protocolo: DevId inválido")
)
