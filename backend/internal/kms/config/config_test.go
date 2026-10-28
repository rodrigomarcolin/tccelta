package config

import "testing"

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestPadroes(t *testing.T) {
	c, err := CarregarDe(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if c.HTTP.Endereco != ":8300" || c.Vault.MountAuth != "transit-auth" || c.Rotacao.Periodo.Hours() != 720 || !c.Dev() {
		t.Fatalf("%+v", c)
	}
}

func TestInvalidos(t *testing.T) {
	if _, err := CarregarDe(env(map[string]string{"AMBIENTE": "xx"})); err == nil {
		t.Fatal("ambiente inválido")
	}
	if _, err := CarregarDe(env(map[string]string{"VAULT_TIMEOUT": "abc"})); err == nil {
		t.Fatal("duração inválida")
	}
}

func TestMTLSObrigatorioForaDoDev(t *testing.T) {
	dev, _ := CarregarDe(env(map[string]string{"VAULT_ADDR": "http://vault:8200"}))
	if err := dev.ValidarServidor(); err != nil {
		t.Fatalf("dev aceita HTTP simples: %v", err)
	}
	prod, _ := CarregarDe(env(map[string]string{"AMBIENTE": "prodlike", "VAULT_ADDR": "https://vault:8200"}))
	if err := prod.ValidarServidor(); err == nil {
		t.Fatal("prodlike exige mTLS")
	}
	prod, _ = CarregarDe(env(map[string]string{"AMBIENTE": "prodlike", "VAULT_ADDR": "https://vault:8200",
		"KMS_TLS_CERT": "a", "KMS_TLS_KEY": "b", "KMS_TLS_CLIENT_CA": "c"}))
	if err := prod.ValidarServidor(); err != nil {
		t.Fatal(err)
	}
	if err := (Config{}).ValidarServidor(); err == nil {
		t.Fatal("sem VAULT_ADDR")
	}
}
