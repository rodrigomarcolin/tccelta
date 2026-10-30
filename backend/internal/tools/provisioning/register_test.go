package provisioning

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// goldenRecord é o record.json que `tccelta-kms provisionar` grava; o teste do módulo kms confere
// que ele produz exatamente este texto (contrato em docs/kms-dongle-record.md).
const goldenRecord = `{
  "schema": "tccelta.dongle.record.v1",
  "dev_id": "0011223344556677",
  "profile": "hmac",
  "kms_auth_key": "0011223344556677",
  "kms_rec_key": "0011223344556677",
  "description": "dongle de teste"
}
`

func write(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReadRecordFromKMS(t *testing.T) {
	r, err := ReadRecord(write(t, t.TempDir(), "record.json", goldenRecord))
	if err != nil {
		t.Fatal(err)
	}
	if r.DevID != "0011223344556677" || r.Profile != "hmac" || r.KMSAuthKey != "0011223344556677" || r.Description != "dongle de teste" {
		t.Fatalf("%+v", r)
	}
}

func TestAsymRecordRequiresPublicKey(t *testing.T) {
	base := strings.Replace(goldenRecord, `"profile": "hmac"`, `"profile": "asym"`, 1)
	if _, err := ReadRecord(write(t, t.TempDir(), "record.json", base)); err == nil {
		t.Fatal("asym sem public_key deveria falhar")
	}
	with := strings.Replace(base, `"description"`, `"public_key": "04`+strings.Repeat("ab", 64)+`", "description"`, 1)
	if _, err := ReadRecord(write(t, t.TempDir(), "record.json", with)); err != nil {
		t.Fatal(err)
	}
}

func TestInvalidRecord(t *testing.T) {
	usecases := map[string]string{
		"schema errado":   strings.Replace(goldenRecord, "record.v1", "record.v9", 1),
		"dev_id curto":    strings.Replace(goldenRecord, "0011223344556677", "0011", 1),
		"perfil inválido": strings.Replace(goldenRecord, `"hmac"`, `"rsa"`, 1),
		"sem chave auth":  strings.Replace(goldenRecord, `"kms_auth_key": "0011223344556677"`, `"kms_auth_key": ""`, 1),
		"campo extra":     strings.Replace(goldenRecord, `"profile"`, `"segredo": "x", "profile"`, 1),
		"json quebrado":   "{",
	}
	for name, c := range usecases {
		if _, err := ReadRecord(write(t, t.TempDir(), "record.json", c)); err == nil {
			t.Errorf("%s: deveria falhar", name)
		}
	}
}

func TestRecordFiles(t *testing.T) {
	dir := t.TempDir()
	a := write(t, dir, "0011223344556677/record.json", goldenRecord)
	b := write(t, dir, "0011223344556678/record.json", strings.ReplaceAll(goldenRecord, "0011223344556677", "0011223344556678"))
	write(t, dir, "0011223344556678/dongle.json", "{}")
	got, err := RecordFiles(dir)
	if err != nil || len(got) != 2 || got[0] != a || got[1] != b {
		t.Fatalf("%v %v", err, got)
	}
	if got, _ := RecordFiles(a); len(got) != 1 || got[0] != a {
		t.Fatalf("%v", got)
	}
	if _, err := RecordFiles(filepath.Join(dir, "nao-existe")); err == nil {
		t.Fatal("caminho inexistente")
	}
}

func TestRegistration(t *testing.T) {
	r, err := ReadRecord(write(t, t.TempDir(), "record.json", goldenRecord))
	if err != nil {
		t.Fatal(err)
	}
	g, err := r.Registration()
	if err != nil {
		t.Fatal(err)
	}
	if g.DevID.Hex() != "0011223344556677" || g.Profile != "hmac" || g.AuthKey != "0011223344556677" || g.RecKey != "0011223344556677" ||
		g.Description != "dongle de teste" || g.PublicKey != nil {
		t.Fatalf("%+v", g)
	}
	r.PublicKey, r.Profile = "04"+strings.Repeat("ab", 64), "asym"
	if g, err := r.Registration(); err != nil || len(g.PublicKey) != 65 || g.PublicKey[0] != 4 {
		t.Fatalf("%+v %v", g, err)
	}
	r.Profile = "rsa"
	if _, err := r.Registration(); err == nil {
		t.Fatal("record inválido não vira cadastro")
	}
}
