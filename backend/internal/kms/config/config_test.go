package config

import "testing"

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestDefaults(t *testing.T) {
	c, err := LoadFrom(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if c.HTTP.Address != ":8300" || c.Vault.MountAuth != "transit-auth" || c.Rotation.Period.Hours() != 720 || !c.Dev() {
		t.Fatalf("%+v", c)
	}
}

func TestInvalid(t *testing.T) {
	if _, err := LoadFrom(env(map[string]string{"AMBIENTE": "xx"})); err == nil {
		t.Fatal("ambiente inválido")
	}
	if _, err := LoadFrom(env(map[string]string{"VAULT_TIMEOUT": "abc"})); err == nil {
		t.Fatal("duração inválida")
	}
}

func TestMTLSRequiredOutsideDev(t *testing.T) {
	dev, _ := LoadFrom(env(map[string]string{"VAULT_ADDR": "http://vault:8200"}))
	if err := dev.ValidateServer(); err != nil {
		t.Fatalf("dev aceita HTTP simples: %v", err)
	}
	prod, _ := LoadFrom(env(map[string]string{"AMBIENTE": "prodlike", "VAULT_ADDR": "https://vault:8200"}))
	if err := prod.ValidateServer(); err == nil {
		t.Fatal("prodlike exige mTLS")
	}
	prod, _ = LoadFrom(env(map[string]string{"AMBIENTE": "prodlike", "VAULT_ADDR": "https://vault:8200",
		"KMS_TLS_CERT": "a", "KMS_TLS_KEY": "b", "KMS_TLS_CLIENT_CA": "c"}))
	if err := prod.ValidateServer(); err != nil {
		t.Fatal(err)
	}
	if err := (Config{}).ValidateServer(); err == nil {
		t.Fatal("sem VAULT_ADDR")
	}
}
