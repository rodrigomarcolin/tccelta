package config

import (
	"os"
	"path/filepath"
	"testing"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestDefaults(t *testing.T) {
	c, err := LoadFrom(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if c.Protocol.MaxBatchBlocks != 32 || c.JWT.Validity.Hours() != 1 || c.Environment != "dev" {
		t.Fatalf("%+v", c)
	}
}

func TestSecretFile(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "s")
	_ = os.WriteFile(p, []byte("abc\n"), 0o600)
	c, err := LoadFrom(env(map[string]string{"BANCO_SENHA_ARQUIVO": p}))
	if err != nil || c.Database.Password != "abc" {
		t.Fatalf("%v %q", err, c.Database.Password)
	}
	if _, err := LoadFrom(env(map[string]string{"BANCO_SENHA_ARQUIVO": filepath.Join(d, "nao")})); err == nil {
		t.Fatal("esperava erro")
	}
}

func TestProductionRefusesNone(t *testing.T) {
	if _, err := LoadFrom(env(map[string]string{"AMBIENTE": "producao", "PERMITIR_PERFIL_NONE": "true"})); err == nil {
		t.Fatal("esperava erro")
	}
	if _, err := LoadFrom(env(map[string]string{"AMBIENTE": "xx"})); err == nil {
		t.Fatal("ambiente inválido")
	}
	if _, err := LoadFrom(env(map[string]string{"JWT_VALIDADE": "abc"})); err == nil {
		t.Fatal("duração inválida")
	}
}

func TestKMS(t *testing.T) {
	c, err := LoadFrom(env(map[string]string{"KMS_URL": "http://kms:8300", "KMS_TIMEOUT": "500ms", "KMS_MAX_CONCORRENCIA": "8"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.KMS.URL != "http://kms:8300" || c.KMS.Timeout.Milliseconds() != 500 || c.KMS.MaxConcurrency != 8 || c.KMS.VerifyBatch != 32 {
		t.Fatalf("%+v", c.KMS)
	}
}
