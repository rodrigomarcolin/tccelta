package simulator

import (
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"math/big"
	"os"
	"strings"

	"tccelta/backend/internal/domain/protocol"
)

// ReadKeysCSV lê dev_id_hex,k_auth_hex,k_rec_hex,perfil[,k_sig_hex] (dongles de teste do simulador).
// A 5ª coluna é obrigatória no perfil asym e ignorada (se vazia) nos demais.
func ReadKeysCSV(file string) ([]Keys, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	rd := csv.NewReader(f)
	rd.Comment = '#'
	rd.FieldsPerRecord = -1
	recs, err := rd.ReadAll()
	if err != nil {
		return nil, err
	}
	var out []Keys
	for _, r := range recs {
		if len(r) != 4 && len(r) != 5 {
			return nil, fmt.Errorf("linha inválida %q: esperadas 4 ou 5 colunas", r[0])
		}
		var c Keys
		dev, e1 := hex.DecodeString(strings.TrimSpace(r[0]))
		ka, e2 := hex.DecodeString(strings.TrimSpace(r[1]))
		kr, e3 := hex.DecodeString(strings.TrimSpace(r[2]))
		if e1 != nil || e2 != nil || e3 != nil || len(dev) != 8 {
			return nil, fmt.Errorf("linha inválida %q", r[0])
		}
		prof, err := protocol.ProfileByName(strings.TrimSpace(r[3]))
		if err != nil {
			return nil, fmt.Errorf("linha %q: %w", r[0], err)
		}
		copy(c.Dev[:], dev)
		c.KAuth, c.KRec, c.Profile = ka, kr, prof
		if prof.Name == protocol.ProfileAsym.Name {
			if len(r) < 5 {
				return nil, fmt.Errorf("linha %q: perfil asym exige k_sig_hex", r[0])
			}
			if c.KSig, err = parseKSig(strings.TrimSpace(r[4])); err != nil {
				return nil, fmt.Errorf("linha %q: %w", r[0], err)
			}
		}
		out = append(out, c)
	}
	return out, nil
}

// parseKSig monta a chave privada P-256 a partir do escalar de 32 B (big-endian).
func parseKSig(h string) (*ecdsa.PrivateKey, error) {
	raw, err := hex.DecodeString(h)
	if err != nil || len(raw) != 32 {
		return nil, fmt.Errorf("k_sig_hex inválida")
	}
	k, err := ecdh.P256().NewPrivateKey(raw)
	if err != nil {
		return nil, fmt.Errorf("k_sig_hex inválida: %w", err)
	}
	pub := k.PublicKey().Bytes() // 04 ‖ X ‖ Y
	return &ecdsa.PrivateKey{
		PublicKey: ecdsa.PublicKey{Curve: elliptic.P256(), X: new(big.Int).SetBytes(pub[1:33]), Y: new(big.Int).SetBytes(pub[33:])},
		D:         new(big.Int).SetBytes(raw),
	}, nil
}
