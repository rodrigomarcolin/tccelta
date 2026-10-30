package protocol

import (
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"errors"
	"math/big"
)

// PublicKeySize é o tamanho da chave pública P-256 não comprimida (04 ‖ X ‖ Y).
const PublicKeySize = 65

// SignatureSize é o tamanho da tag asym: r(32) ‖ s(32), big-endian.
const SignatureSize = 64

// ErrInvalidPublicKey: a chave pública do cadastro não é um ponto P-256 válido.
var ErrInvalidPublicKey = errors.New("protocolo: chave pública inválida")

// ParsePublicKey valida os 65 B não comprimidos e o ponto na curva.
func ParsePublicKey(raw []byte) (*ecdsa.PublicKey, error) {
	if len(raw) != PublicKeySize || raw[0] != 0x04 {
		return nil, ErrInvalidPublicKey
	}
	if _, err := ecdh.P256().NewPublicKey(raw); err != nil {
		return nil, ErrInvalidPublicKey
	}
	return &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     new(big.Int).SetBytes(raw[1:33]),
		Y:     new(big.Int).SetBytes(raw[33:]),
	}, nil
}

// VerifyBlockSignature confere a tag ECDSA P-256 (r‖s) sobre SHA-256(input).
// Aceita s alto e baixo (sem normalização low-S).
func VerifyBlockSignature(pub *ecdsa.PublicKey, input, tag []byte) bool {
	if pub == nil || len(tag) != SignatureSize {
		return false
	}
	r := new(big.Int).SetBytes(tag[:32])
	s := new(big.Int).SetBytes(tag[32:])
	h := sha256.Sum256(input)
	return ecdsa.Verify(pub, h[:], r, s) // recusa r, s fora de [1, n-1]
}
