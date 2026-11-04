package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestTripDefaults(t *testing.T) {
	c, err := LoadFrom(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	w := c.Worker
	if w.TripWaitNoBatch != 10*time.Second || w.TripFinalLimit != 20*time.Second || w.TripEvalInterval != time.Second ||
		w.TripProvisionalInterval != 5*time.Second || w.TripCalcTimeout != 60*time.Second {
		t.Fatalf("%+v", w)
	}
}

func TestTripOverrides(t *testing.T) {
	c, err := LoadFrom(env(map[string]string{"VIAGEM_ESPERA_SEM_LOTE": "3s", "VIAGEM_LIMITE_FINAL": "8s", "VIAGEM_INTERVALO_AVALIACAO": "500ms",
		"VIAGEM_INTERVALO_PROVISORIAS": "2s", "VIAGEM_TIMEOUT_CALCULO": "90s"}))
	if err != nil {
		t.Fatal(err)
	}
	w := c.Worker
	if w.TripWaitNoBatch != 3*time.Second || w.TripFinalLimit != 8*time.Second || w.TripEvalInterval != 500*time.Millisecond ||
		w.TripProvisionalInterval != 2*time.Second || w.TripCalcTimeout != 90*time.Second {
		t.Fatalf("%+v", w)
	}
}

func TestTripInvalid(t *testing.T) {
	for name, m := range map[string]map[string]string{
		"valor zero":                  {"VIAGEM_INTERVALO_PROVISORIAS": "0s"},
		"valor negativo":              {"VIAGEM_TIMEOUT_CALCULO": "-1s"},
		"limite final <= espera":      {"VIAGEM_ESPERA_SEM_LOTE": "20s", "VIAGEM_LIMITE_FINAL": "20s"},
		"limite final < espera":       {"VIAGEM_ESPERA_SEM_LOTE": "30s"},
		"avaliação > espera sem lote": {"VIAGEM_INTERVALO_AVALIACAO": "11s"},
		"duração ilegível":            {"VIAGEM_LIMITE_FINAL": "vinte"},
	} {
		if _, err := LoadFrom(env(m)); err == nil {
			t.Errorf("%s: esperava erro", name)
		}
	}
}

func TestAnalysisNoAuth(t *testing.T) {
	c, err := LoadFrom(env(nil))
	if err != nil || c.AnalysisNoAuth || c.Limits.TripIPMin != 600 {
		t.Fatalf("padrão: %v %+v", err, c)
	}
	if _, err := LoadFrom(env(map[string]string{"AMBIENTE": "producao", "ANALISE_SEM_AUTH": "true"})); err == nil ||
		!strings.Contains(err.Error(), "ANALISE_SEM_AUTH é proibido em AMBIENTE=producao") {
		t.Fatalf("produção deve recusar: %v", err)
	}
	for _, amb := range []string{"dev", "prodlike"} {
		c, err := LoadFrom(env(map[string]string{"AMBIENTE": amb, "ANALISE_SEM_AUTH": "true", "LIMITE_VIAGEM_IP_MIN": "50"}))
		if err != nil || !c.AnalysisNoAuth || c.Limits.TripIPMin != 50 {
			t.Fatalf("%s: %v %+v", amb, err, c)
		}
	}
	if _, err := LoadFrom(env(map[string]string{"AMBIENTE": "producao", "ANALISE_SEM_AUTH": "false"})); err != nil {
		t.Fatalf("produção sem o modo: %v", err)
	}
}
