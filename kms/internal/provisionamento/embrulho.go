// Package provisionamento gera/importa as chaves dos dongles no Vault (BYOK) e no cadastro.
package provisionamento

import (
	"bytes"
	"crypto/aes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"errors"
)

var aivPrefixo = []byte{0xA6, 0x59, 0x59, 0xA6}

// Embrulhar implementa o formato BYOK do Vault Transit: RSA-OAEP-SHA256(K_e) || AES-KWP(K_e, chave).
func Embrulhar(wrappingPub *rsa.PublicKey, chave []byte) (string, error) {
	ke := make([]byte, 32)
	if _, err := rand.Read(ke); err != nil {
		return "", err
	}
	defer zerar(ke)
	c1, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, wrappingPub, ke, nil)
	if err != nil {
		return "", err
	}
	c2, err := aesKWP(ke, chave)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(append(c1, c2...)), nil
}

func zerar(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// aesKWP: RFC 5649 (key wrap with padding).
func aesKWP(kek, chave []byte) ([]byte, error) {
	if len(chave) == 0 {
		return nil, errors.New("chave vazia")
	}
	blk, err := aes.NewCipher(kek)
	if err != nil {
		return nil, err
	}
	n := len(chave)
	pad := (8 - n%8) % 8
	p := make([]byte, n+pad)
	copy(p, chave)
	aiv := make([]byte, 8)
	copy(aiv, aivPrefixo)
	binary.BigEndian.PutUint32(aiv[4:], uint32(n))
	if len(p) == 8 {
		b := make([]byte, 16)
		copy(b, aiv)
		copy(b[8:], p)
		out := make([]byte, 16)
		blk.Encrypt(out, b)
		return out, nil
	}
	// RFC 3394 com IV = AIV
	r := make([][]byte, len(p)/8)
	for i := range r {
		r[i] = append([]byte(nil), p[i*8:(i+1)*8]...)
	}
	a := append([]byte(nil), aiv...)
	nn := len(r)
	buf := make([]byte, 16)
	for j := 0; j < 6; j++ {
		for i := 0; i < nn; i++ {
			copy(buf, a)
			copy(buf[8:], r[i])
			blk.Encrypt(buf, buf)
			t := uint64(nn*j + i + 1)
			copy(a, buf[:8])
			var tb [8]byte
			binary.BigEndian.PutUint64(tb[:], t)
			for k := 0; k < 8; k++ {
				a[k] ^= tb[k]
			}
			copy(r[i], buf[8:])
		}
	}
	out := append([]byte(nil), a...)
	for _, x := range r {
		out = append(out, x...)
	}
	return out, nil
}

// aesKWPDesembrulhar é a operação inversa (usada nos testes).
func aesKWPDesembrulhar(kek, c []byte) ([]byte, error) {
	blk, err := aes.NewCipher(kek)
	if err != nil {
		return nil, err
	}
	var a []byte
	var p []byte
	if len(c) == 16 {
		b := make([]byte, 16)
		blk.Decrypt(b, c)
		a, p = b[:8], b[8:]
	} else {
		nn := len(c)/8 - 1
		a = append([]byte(nil), c[:8]...)
		r := make([][]byte, nn)
		for i := range r {
			r[i] = append([]byte(nil), c[(i+1)*8:(i+2)*8]...)
		}
		buf := make([]byte, 16)
		for j := 5; j >= 0; j-- {
			for i := nn - 1; i >= 0; i-- {
				t := uint64(nn*j + i + 1)
				var tb [8]byte
				binary.BigEndian.PutUint64(tb[:], t)
				copy(buf, a)
				for k := 0; k < 8; k++ {
					buf[k] ^= tb[k]
				}
				copy(buf[8:], r[i])
				blk.Decrypt(buf, buf)
				copy(a, buf[:8])
				copy(r[i], buf[8:])
			}
		}
		p = bytes.Join(r, nil)
	}
	if subtle.ConstantTimeCompare(a[:4], aivPrefixo) != 1 {
		return nil, errors.New("AIV inválido")
	}
	n := int(binary.BigEndian.Uint32(a[4:]))
	if n > len(p) || len(p)-n >= 8 {
		return nil, errors.New("MLI inválido")
	}
	for _, x := range p[n:] {
		if x != 0 {
			return nil, errors.New("padding inválido")
		}
	}
	return p[:n], nil
}
