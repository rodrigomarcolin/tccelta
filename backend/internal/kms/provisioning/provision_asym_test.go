package provisioning

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"encoding/hex"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var p256Order, _ = new(big.Int).SetString("FFFFFFFF00000000FFFFFFFFFFFFFFFFBCE6FAADA7179E84F3B9CAC2FC632551", 16)

func TestNewAsymGeneratesConsistentKeyPair(t *testing.T) {
	f := newFake(t)
	d, r, err := newProv(f, "dev").NewDongle(context.Background(), "asym")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.KSig) != 32 {
		t.Fatalf("k_sig com %d B", len(d.KSig))
	}
	if k := new(big.Int).SetBytes(d.KSig); k.Sign() <= 0 || k.Cmp(p256Order) >= 0 {
		t.Fatal("k_sig fora de [1, n-1]")
	}
	pub, err := d.PublicKey()
	if err != nil || len(pub) != 65 || pub[0] != 0x04 {
		t.Fatalf("%x %v", pub, err)
	}
	if r.PublicKey != strings.ToUpper(hex.EncodeToString(pub)) {
		t.Fatalf("record.public_key = %s", r.PublicKey)
	}
	if _, err := ecdh.P256().NewPublicKey(pub); err != nil {
		t.Fatalf("ponto fora da curva: %v", err)
	}
	if !f.keys["transit-auth/"+r.DevID] || !f.keys["transit-rec/"+r.DevID] {
		t.Fatal("K_auth e K_rec continuam importadas (D5)")
	}
	if !bytes.Contains(CSVSecnvs(d), []byte("k_sig,data,hex2bin,"+hex.EncodeToString(d.KSig))) {
		t.Fatal("NVS sem k_sig")
	}
}

func TestNewHMACHasNoSigningKey(t *testing.T) {
	d, r, err := newProv(newFake(t), "dev").NewDongle(context.Background(), "hmac")
	if err != nil {
		t.Fatal(err)
	}
	if d.KSig != nil || r.PublicKey != "" {
		t.Fatalf("%x %q", d.KSig, r.PublicKey)
	}
}

func TestImportWithKeysAsymRequiresKSig(t *testing.T) {
	d := dongle(t)
	d.Profile = "asym"
	f := newFake(t)
	if _, err := newProv(f, "dev").ImportWithKeys(context.Background(), d, ""); err == nil {
		t.Fatal("asym sem k_sig deveria ser recusado")
	}
	if len(f.imported) != 0 {
		t.Fatal("nada pode ser importado no Vault antes da validação")
	}
}

func writeCSV(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "k.csv")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

const (
	csvDev = "0011223344556677"
	csvKA  = "101112131415161718191a1b1c1d1e1f202122232425262728292a2b2c2d2e2f"
	csvKR  = "505152535455565758595a5b5c5d5e5f606162636465666768696a6b6c6d6e6f"
	csvSig = "2222222222222222222222222222222222222222222222222222222222222222"
)

func TestImportCSVColumns(t *testing.T) {
	line := func(prof, sig string) string {
		l := csvDev + "," + csvKA + "," + csvKR + "," + prof
		if sig != "-" {
			l += "," + sig
		}
		return l + "\n"
	}
	cases := []struct {
		name, body string
		ok         bool
		pub        bool
	}{
		{"4 colunas hmac", line("hmac", "-"), true, false},
		{"5 colunas hmac com a última vazia", line("hmac", ""), true, false},
		{"5 colunas asym", line("asym", csvSig), true, true},
		{"asym sem k_sig_hex", line("asym", "-"), false, false},
		{"asym com a coluna vazia", line("asym", ""), false, false},
		{"hmac com k_sig_hex", line("hmac", csvSig), false, false},
		{"none com k_sig_hex", line("none", csvSig), false, false},
		{"k_sig_hex curta", line("asym", "2222"), false, false},
		{"k_sig_hex zero", line("asym", strings.Repeat("00", 32)), false, false},
		{"6 colunas", line("asym", csvSig+",x"), false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, rs, err := newProv(newFake(t), "dev").ImportCSV(context.Background(), writeCSV(t, c.body))
			if (err == nil) != c.ok {
				t.Fatalf("err = %v, ok esperado = %v", err, c.ok)
			}
			if c.ok && (rs[0].PublicKey != "") != c.pub {
				t.Fatalf("public_key = %q", rs[0].PublicKey)
			}
		})
	}
}

func TestImportCSVAsymDerivesPublicKeyFromKSig(t *testing.T) {
	_, rs, err := newProv(newFake(t), "dev").ImportCSV(context.Background(),
		writeCSV(t, csvDev+","+csvKA+","+csvKR+",asym,"+csvSig+"\n"))
	if err != nil {
		t.Fatal(err)
	}
	k, _ := hex.DecodeString(csvSig)
	priv, err := ecdh.P256().NewPrivateKey(k)
	if err != nil {
		t.Fatal(err)
	}
	if want := strings.ToUpper(hex.EncodeToString(priv.PublicKey().Bytes())); rs[0].PublicKey != want {
		t.Fatalf("public_key = %s, esperado %s", rs[0].PublicKey, want)
	}
}
