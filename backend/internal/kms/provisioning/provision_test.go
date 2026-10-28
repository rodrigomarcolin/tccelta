package provisioning

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tccelta/backend/internal/kms/vault"
)

// fakeVault guarda quais chaves existem e quantas importações aconteceram.
type fakeVault struct {
	pub      string
	keys     map[string]bool // "<mount>/<nome>"
	imported []string
}

func newFake(t *testing.T) *fakeVault {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, _ := x509.MarshalPKIXPublicKey(&k.PublicKey)
	return &fakeVault{pub: string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})), keys: map[string]bool{}}
}

func (f *fakeVault) Read(_ context.Context, path string) (map[string]any, error) {
	if strings.HasSuffix(path, "/wrapping_key") {
		return map[string]any{"data": map[string]any{"public_key": f.pub}}, nil
	}
	if p := strings.SplitN(path, "/keys/", 2); len(p) == 2 && f.keys[p[0]+"/"+p[1]] {
		return map[string]any{"data": map[string]any{"type": "hmac"}}, nil
	}
	return nil, &vault.VaultError{Status: 404, Messages: []string{"não encontrada"}}
}

func (f *fakeVault) Write(_ context.Context, path string, body map[string]any) (map[string]any, error) {
	p := strings.Split(path, "/") // <mount>/keys/<nome>/import
	if len(p) != 4 || p[1] != "keys" || p[3] != "import" || body["ciphertext"] == "" || body["exportable"] != false {
		return nil, &vault.VaultError{Status: 400, Messages: []string{"requisição inesperada " + path}}
	}
	f.keys[p[0]+"/"+p[2]] = true
	f.imported = append(f.imported, p[0]+"/"+p[2])
	return map[string]any{}, nil
}

func newProv(f *fakeVault, environment string) *Provisioner {
	return &Provisioner{T: f, MountAuth: "transit-auth", MountRec: "transit-rec", Environment: environment,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func dongle(t *testing.T) Dongle {
	t.Helper()
	d := Dongle{DevID: make([]byte, 8), KAuth: make([]byte, 32), KRec: make([]byte, 32), Profile: "hmac"}
	for _, b := range [][]byte{d.DevID, d.KAuth, d.KRec} {
		_, _ = rand.Read(b)
	}
	return d
}

func TestImportWithKeysIsIdempotent(t *testing.T) {
	f := newFake(t)
	p := newProv(f, "dev")
	d := dongle(t)
	hx := hex.EncodeToString(d.DevID)
	r1, err := p.ImportWithKeys(context.Background(), d, "teste")
	if err != nil {
		t.Fatal(err)
	}
	if r1.Schema != RecordSchema || r1.DevID != hx || r1.Profile != "hmac" || r1.KMSAuthKey != hx || r1.KMSRecKey != hx || r1.Description != "teste" {
		t.Fatalf("%+v", r1)
	}
	if len(f.imported) != 2 {
		t.Fatalf("esperava 2 importações, houve %v", f.imported)
	}
	// reexecução: pula o import e reemite o mesmo registro
	r2, err := p.ImportWithKeys(context.Background(), d, "teste")
	if err != nil || r2 != r1 || len(f.imported) != 2 {
		t.Fatalf("%v %+v importações=%v", err, r2, f.imported)
	}
}

func TestInterruptedImportCompletesWhatIsMissing(t *testing.T) {
	f := newFake(t)
	p := newProv(f, "dev")
	d := dongle(t)
	f.keys["transit-auth/"+hex.EncodeToString(d.DevID)] = true // só K_auth chegou a ser importada
	if _, err := p.ImportWithKeys(context.Background(), d, ""); err != nil {
		t.Fatal(err)
	}
	if len(f.imported) != 1 || !strings.HasPrefix(f.imported[0], "transit-rec/") {
		t.Fatalf("deveria importar só K_rec: %v", f.imported)
	}
}

func TestNewPicksFreeDevIDFromVaultOnly(t *testing.T) {
	f := newFake(t)
	p := newProv(f, "dev")
	d, r, err := p.NewDongle(context.Background(), "hmac")
	if err != nil {
		t.Fatal(err)
	}
	if r.DevID != hex.EncodeToString(d.DevID) || len(d.KAuth) != 32 || len(d.KRec) != 32 || !f.keys["transit-auth/"+r.DevID] || !f.keys["transit-rec/"+r.DevID] {
		t.Fatalf("%+v %v", r, f.keys)
	}
	if _, _, err := newProv(f, "producao").NewDongle(context.Background(), "hmac"); err == nil {
		t.Fatal("novo deve ser recusado em producao")
	}
}

func TestImportCSV(t *testing.T) {
	f := newFake(t)
	p := newProv(f, "dev")
	csv := filepath.Join(t.TempDir(), "d.csv")
	line := func(dev string) string {
		return dev + "," + strings.Repeat("11", 32) + "," + strings.Repeat("22", 32) + ",hmac\n"
	}
	_ = os.WriteFile(csv, []byte("# comentário\n"+line("0011223344556677")+line("0011223344556678")), 0o600)
	ds, rs, err := p.ImportCSV(context.Background(), csv)
	if err != nil || len(ds) != 2 || len(rs) != 2 || len(f.imported) != 4 {
		t.Fatalf("%v %d %d %v", err, len(ds), len(rs), f.imported)
	}
	if _, rs2, err := p.ImportCSV(context.Background(), csv); err != nil || len(rs2) != 2 || len(f.imported) != 4 {
		t.Fatalf("reexecução deveria ser idempotente: %v %v", err, f.imported)
	}
	if _, _, err := newProv(f, "producao").ImportCSV(context.Background(), csv); err == nil {
		t.Fatal("CSV de teste é recusado em producao")
	}
}

// goldenRecord é o record.json que o backend (`tccelta provision register`) deve aceitar;
// o teste correspondente no backend usa o mesmo texto (contrato em docs/kms-dongle-record.md).
const goldenRecord = `{
  "schema": "tccelta.dongle.record.v1",
  "dev_id": "0011223344556677",
  "profile": "hmac",
  "kms_auth_key": "0011223344556677",
  "kms_rec_key": "0011223344556677",
  "description": "dongle de teste"
}
`

func TestRecordWrittenInContractFormat(t *testing.T) {
	r := Record{Schema: RecordSchema, DevID: "0011223344556677", Profile: "hmac", KMSAuthKey: "0011223344556677",
		KMSRecKey: "0011223344556677", Description: "dongle de teste"}
	dir := t.TempDir()
	path, err := r.Save(dir)
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(dir, "0011223344556677", "record.json") {
		t.Fatal(path)
	}
	got, _ := os.ReadFile(path)
	if string(got) != goldenRecord {
		t.Fatalf("record.json difere do contrato:\n%s", got)
	}
}
