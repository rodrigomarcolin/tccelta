package provisioning

import (
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// As fixtures de testdata/provision_py saíram do esp32-firmware/tools/provision/provision.py
// (--profile dev, com e sem --kms-wrapping), usando a wrapping key RSA de teste wrapping_priv.pem.
const pyFixtures = "testdata/provision_py"

type benchKeys struct {
	DevID string `json:"dev_id"`
	KAuth string `json:"k_auth"`
	KRec  string `json:"k_rec"`
}

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatal(err)
	}
}

// fakeWithFixtureWrapping serve a wrapping key do kms_wrapping.json e devolve a privada para desembrulhar.
func fakeWithFixtureWrapping(t *testing.T) (*fakeVault, *rsa.PrivateKey) {
	t.Helper()
	var w struct {
		Schema string `json:"schema"`
		Auth   struct {
			Kid string `json:"kid"`
			PEM string `json:"pem"`
		} `json:"auth"`
	}
	readJSON(t, filepath.Join(pyFixtures, "kms_wrapping.json"), &w)
	raw, err := os.ReadFile(filepath.Join(pyFixtures, "wrapping_priv.pem"))
	if err != nil {
		t.Fatal(err)
	}
	blk, _ := pem.Decode(raw)
	k, err := x509.ParsePKCS8PrivateKey(blk.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	f := newFake(t)
	f.pub = w.Auth.PEM
	return f, k.(*rsa.PrivateKey)
}

func unwrapBYOK(t *testing.T, priv *rsa.PrivateKey, ct string) []byte {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(ct)
	if err != nil {
		t.Fatal(err)
	}
	ke, err := rsa.DecryptOAEP(sha256.New(), nil, priv, raw[:priv.Size()], nil)
	if err != nil {
		t.Fatal(err)
	}
	key, err := aesKWPUnwrap(ke, raw[priv.Size():])
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func assertImportedKeys(t *testing.T, f *fakeVault, priv *rsa.PrivateKey, dir string) {
	t.Helper()
	var bk benchKeys
	readJSON(t, filepath.Join(dir, "bench_keys.json"), &bk)
	dev := strings.ToLower(bk.DevID)
	for mount, want := range map[string]string{"transit-auth": bk.KAuth, "transit-rec": bk.KRec} {
		ct, ok := f.ciphertexts[mount+"/"+dev]
		if !ok {
			t.Fatalf("%s/%s não foi importada", mount, dev)
		}
		if got := hex.EncodeToString(unwrapBYOK(t, priv, ct)); !strings.EqualFold(got, want) {
			t.Fatalf("%s: o Vault receberia %s, o firmware tem %s", mount, got, want)
		}
	}
}

func TestImportRecordFromProvisionPyBYOK(t *testing.T) {
	f, priv := fakeWithFixtureWrapping(t)
	dir := filepath.Join(pyFixtures, "byok_asym")
	r, err := newProv(f, "producao").ImportRecord(context.Background(), filepath.Join(dir, "kms_record.json"), "bancada")
	if err != nil {
		t.Fatalf("o kid do kms_wrapping.json tem de bater com o da wrapping key: %v", err)
	}
	if r.DevID != "0011223344556677" || r.Profile != "asym" || r.KMSAuthKey != r.DevID || r.KMSRecKey != r.DevID {
		t.Fatalf("%+v", r)
	}
	assertImportedKeys(t, f, priv, dir)

	var env EnvelopeKMS
	readJSON(t, filepath.Join(dir, "kms_record.json"), &env)
	if r.PublicKey != env.KSigPublic {
		t.Fatalf("public_key %s, k_sig_public %s", r.PublicKey, env.KSigPublic)
	}
	csv, err := os.ReadFile(filepath.Join(dir, "secnvs.csv"))
	if err != nil {
		t.Fatal(err)
	}
	var kSig []byte
	for _, line := range strings.Split(string(csv), "\n") {
		if v, ok := strings.CutPrefix(line, "k_sig,data,hex2bin,"); ok {
			kSig, _ = hex.DecodeString(strings.TrimSpace(v))
		}
	}
	pub, err := Dongle{KSig: kSig}.PublicKey()
	if err != nil || !strings.EqualFold(hex.EncodeToString(pub), r.PublicKey) {
		t.Fatalf("k_sig da NVS não gera a k_sig_public do envelope: %x %v", pub, err)
	}
}

func TestImportRecordFromProvisionPyAlgNone(t *testing.T) {
	f, priv := fakeWithFixtureWrapping(t)
	dir := filepath.Join(pyFixtures, "none_hmac")
	file := filepath.Join(dir, "kms_record.json")
	r, err := newProv(f, "dev").ImportRecord(context.Background(), file, "")
	if err != nil {
		t.Fatal(err)
	}
	if r.DevID != "0011223344556678" || r.Profile != "hmac" || r.PublicKey != "" {
		t.Fatalf("%+v", r)
	}
	assertImportedKeys(t, f, priv, dir)

	if _, err := newProv(newFake(t), "producao").ImportRecord(context.Background(), file, ""); err == nil {
		t.Fatal("alg none deve ser recusado em producao")
	}
}

func TestImportRecordRejectsOtherWrappingKey(t *testing.T) {
	f := newFake(t) // wrapping key diferente da usada pelo provision.py
	_, err := newProv(f, "dev").ImportRecord(context.Background(), filepath.Join(pyFixtures, "byok_asym", "kms_record.json"), "")
	if err == nil || !strings.Contains(err.Error(), "kid") {
		t.Fatalf("esperava erro de kid, veio %v", err)
	}
	if len(f.imported) != 0 {
		t.Fatalf("nada deveria ter sido importado: %v", f.imported)
	}
}
