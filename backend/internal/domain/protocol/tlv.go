// Package protocol codifica e decodifica, byte a byte, as mensagens 0x01-0x04,
// os registros do bloco e a entrada do HMAC. Só usa a stdlib.
//
// Todos os inteiros são little-endian, exceto os 2 bytes de valor do registro
// de PID (A, B crus da resposta OBD-II).
package protocol

import (
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// Label é o prefixo da entrada do HMAC.
const Label = "MALCONG"

// Type é o byte T do envelope TLV.
type Type uint8

const (
	ChallengeType Type = 0x01
	ResponseType  Type = 0x02
	BlockType     Type = 0x03
	AckType       Type = 0x04
)

// Tamanhos fixos.
const (
	AuthTagSize     = 16
	RecTagSize      = 32
	NonceSize       = 16
	BlockHeaderSize = 12
	MinDataSize     = 17 // 12 de cabeçalho + 1 registro de PID (5)
	MaxDataSize     = 1024
	MarkerSize      = 11
	MarkerBlockSize = BlockHeaderSize + MarkerSize
	LChallenge      = 45
	LChallengeDiag  = 47
	LResponse       = 50
	LAck            = 25
)

// DevID é o identificador de 8 bytes do dongle.
type DevID [8]byte

// DevIDFromHex converte exatamente 16 dígitos hexadecimais.
func DevIDFromHex(s string) (DevID, error) {
	var d DevID
	if len(s) != 16 {
		return d, ErrInvalidDevID
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return d, ErrInvalidDevID
	}
	copy(d[:], b)
	return d, nil
}

// Hex devolve o DevId em minúsculas.
func (d DevID) Hex() string { return hex.EncodeToString(d[:]) }

// TLV é um envelope T | L(u16 LE) | V. V aponta para o buffer original.
type TLV struct {
	T Type
	V []byte
}

// ReadTLV lê um TLV do início de b e devolve os bytes consumidos.
func ReadTLV(b []byte) (TLV, int, error) {
	if len(b) < 3 {
		return TLV{}, 0, ErrTLVShort
	}
	l := int(binary.LittleEndian.Uint16(b[1:3]))
	if len(b) < 3+l {
		return TLV{}, 0, ErrTLVLength
	}
	return TLV{T: Type(b[0]), V: b[3 : 3+l : 3+l]}, 3 + l, nil
}

// ReadBatch lê a concatenação de TLVs; ErrBatchTooLarge se houver mais de max.
func ReadBatch(body []byte, max int) ([]TLV, error) {
	if len(body) == 0 {
		return nil, ErrEmptyBatch
	}
	var out []TLV
	for len(body) > 0 {
		t, n, err := ReadTLV(body)
		if err != nil {
			return nil, err
		}
		if len(out) >= max {
			return nil, ErrBatchTooLarge
		}
		out = append(out, t)
		body = body[n:]
	}
	return out, nil
}

// AppendTLV acrescenta um TLV a dst.
func AppendTLV(dst []byte, t Type, v []byte) []byte {
	dst = append(dst, byte(t), byte(len(v)), byte(len(v)>>8))
	return append(dst, v...)
}

// TagInput monta "MALCONG" || tipo || DevId || payload.
func TagInput(t Type, dev DevID, payload []byte) []byte {
	e := make([]byte, 0, len(Label)+1+8+len(payload))
	e = append(e, Label...)
	e = append(e, byte(t))
	e = append(e, dev[:]...)
	return append(e, payload...)
}

// CompareTag compara em tempo constante; tamanhos diferentes = false.
func CompareTag(computed, received []byte) bool {
	if len(computed) != len(received) {
		return false
	}
	return subtle.ConstantTimeCompare(computed, received) == 1
}

// Profile define só o tamanho da tag do BLOCO; a do handshake é sempre 16 B.
type Profile struct {
	Name         string
	BlockTagSize int
}

var (
	ProfileHMAC = Profile{"hmac", 32}
	ProfileAsym = Profile{"asym", 64}
	ProfileNone = Profile{"none", 0}
)

// ProfileByName devolve o perfil pelo nome do cadastro.
func ProfileByName(n string) (Profile, error) {
	switch strings.ToLower(n) {
	case "hmac":
		return ProfileHMAC, nil
	case "asym":
		return ProfileAsym, nil
	case "none":
		return ProfileNone, nil
	}
	return Profile{}, fmt.Errorf("perfil desconhecido %q", n)
}

// Erros exportados.
var (
	ErrTLVShort         = errors.New("protocolo: TLV curto")
	ErrTLVLength        = errors.New("protocolo: comprimento do TLV inconsistente")
	ErrUnexpectedType   = errors.New("protocolo: tipo inesperado")
	ErrEmptyBatch       = errors.New("protocolo: lote vazio")
	ErrBatchTooLarge    = errors.New("protocolo: lote com TLVs demais")
	ErrInvalidSize      = errors.New("protocolo: tamanho inválido")
	ErrMalformedRecord  = errors.New("protocolo: registro malformado")
	ErrMarkerWithOthers = errors.New("protocolo: marcador junto com outros registros")
	ErrReservedFlags    = errors.New("protocolo: flags reservadas")
	ErrInvalidDevID     = errors.New("protocolo: DevId inválido")
)
