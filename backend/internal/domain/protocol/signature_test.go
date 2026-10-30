package protocol

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"errors"
	"math/big"
	"os"
	"testing"
)

type ecdsaVectors struct {
	DevID   string `json:"dev_id"`
	KSigPub string `json:"k_sig_public"`
	Blocks  []struct {
		Name string `json:"name"`
		Data string `json:"data"`
		Tag  string `json:"ecdsa_tag"`
	} `json:"blocks"`
}

func loadECDSAVectors(t *testing.T) (ecdsaVectors, DevID, *ecdsa.PublicKey) {
	t.Helper()
	raw, err := os.ReadFile("testdata/vetores_firmware.json")
	if err != nil {
		t.Skip("vetores do firmware ausentes")
	}
	var v ecdsaVectors
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	dev, err := DevIDFromHex(v.DevID)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := ParsePublicKey(mustHex(t, v.KSigPub))
	if err != nil {
		t.Fatal(err)
	}
	return v, dev, pub
}

func TestECDSAFirmwareVectors(t *testing.T) {
	v, dev, pub := loadECDSAVectors(t)
	if len(v.Blocks) == 0 {
		t.Fatal("sem blocos nos vetores")
	}
	for _, b := range v.Blocks {
		d, tag := mustHex(t, b.Data), mustHex(t, b.Tag)
		in := TagInput(BlockType, dev, d)
		if !VerifyBlockSignature(pub, in, tag) {
			t.Fatalf("%s: assinatura do firmware não verifica", b.Name)
		}
		bad := append([]byte{}, d...)
		bad[len(bad)-1] ^= 1
		if VerifyBlockSignature(pub, TagInput(BlockType, dev, bad), tag) {
			t.Errorf("%s: bit trocado nos dados verificou", b.Name)
		}
		for _, i := range []int{0, 31, 32, 63} { // r e s
			bt := append([]byte{}, tag...)
			bt[i] ^= 1
			if VerifyBlockSignature(pub, in, bt) {
				t.Errorf("%s: bit %d da tag trocado verificou", b.Name, i)
			}
		}
		for _, n := range []int{63, 65} {
			if VerifyBlockSignature(pub, in, append(append([]byte{}, tag...), make([]byte, 1)...)[:n]) {
				t.Errorf("%s: tag de %d B verificou", b.Name, n)
			}
		}
	}
}

func TestECDSAHighS(t *testing.T) {
	v, dev, pub := loadECDSAVectors(t)
	b := v.Blocks[0]
	in := TagInput(BlockType, dev, mustHex(t, b.Data))
	tag := mustHex(t, b.Tag)
	n := elliptic.P256().Params().N
	s := new(big.Int).SetBytes(tag[32:])
	alt := append([]byte{}, tag[:32]...)
	alt = append(alt, new(big.Int).Sub(n, s).FillBytes(make([]byte, 32))...)
	if !VerifyBlockSignature(pub, in, alt) {
		t.Fatal("s' = n − s deveria verificar (D4)")
	}
}

func TestECDSAZeroScalars(t *testing.T) {
	v, dev, pub := loadECDSAVectors(t)
	b := v.Blocks[0]
	in := TagInput(BlockType, dev, mustHex(t, b.Data))
	tag := mustHex(t, b.Tag)
	rZero := append(make([]byte, 32), tag[32:]...)
	sZero := append(append([]byte{}, tag[:32]...), make([]byte, 32)...)
	if VerifyBlockSignature(pub, in, rZero) || VerifyBlockSignature(pub, in, sZero) {
		t.Fatal("r = 0 ou s = 0 verificou")
	}
	if VerifyBlockSignature(pub, in, make([]byte, 64)) {
		t.Fatal("tag de zeros verificou")
	}
}

func TestParsePublicKey(t *testing.T) {
	good := mustHex(t, "040217E617F0B6443928278F96999E69A23A4F2C152BDF6D6CDF66E5B80282D4ED194A7DEBCB97712D2DDA3CA85AA8765A56F45FC758599652F2897C65306E5794")
	if _, err := ParsePublicKey(good); err != nil {
		t.Fatal(err)
	}
	k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if _, err := ParsePublicKey(append([]byte{0x04}, append(k.X.FillBytes(make([]byte, 32)), k.Y.FillBytes(make([]byte, 32))...)...)); err != nil {
		t.Fatalf("chave gerada recusada: %v", err)
	}
	offCurve := append([]byte{}, good...)
	offCurve[64] ^= 1
	prefix := append([]byte{}, good...)
	prefix[0] = 0x02
	for name, raw := range map[string][]byte{
		"prefixo 0x02":  prefix,
		"64 B":          good[1:],
		"66 B":          append(append([]byte{}, good...), 0),
		"vazia":         nil,
		"fora da curva": offCurve,
	} {
		if _, err := ParsePublicKey(raw); !errors.Is(err, ErrInvalidPublicKey) {
			t.Errorf("%s: err = %v, quer ErrInvalidPublicKey", name, err)
		}
	}
}

func TestVerifyBlockSignatureNilKey(t *testing.T) {
	if VerifyBlockSignature(nil, []byte("x"), make([]byte, 64)) {
		t.Fatal("chave nil verificou")
	}
}
