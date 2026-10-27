package provisionamento

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// registroDourado é o registro.json que `tccelta-kms provisionar` grava; o teste do módulo kms confere
// que ele produz exatamente este texto (contrato em kms/docs/registro-dongle.md).
const registroDourado = `{
  "schema": "tccelta.dongle.registro.v1",
  "dev_id": "0011223344556677",
  "perfil": "hmac",
  "kms_chave_auth": "0011223344556677",
  "kms_chave_rec": "0011223344556677",
  "descricao": "dongle de teste"
}
`

func escrever(t *testing.T, dir, nome, conteudo string) string {
	t.Helper()
	p := filepath.Join(dir, nome)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(conteudo), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLerRegistroDoKMS(t *testing.T) {
	r, err := LerRegistro(escrever(t, t.TempDir(), "registro.json", registroDourado))
	if err != nil {
		t.Fatal(err)
	}
	if r.DevID != "0011223344556677" || r.Perfil != "hmac" || r.KMSChaveAuth != "0011223344556677" || r.Descricao != "dongle de teste" {
		t.Fatalf("%+v", r)
	}
}

func TestRegistroAsymExigeChavePublica(t *testing.T) {
	base := strings.Replace(registroDourado, `"perfil": "hmac"`, `"perfil": "asym"`, 1)
	if _, err := LerRegistro(escrever(t, t.TempDir(), "registro.json", base)); err == nil {
		t.Fatal("asym sem chave_publica deveria falhar")
	}
	com := strings.Replace(base, `"descricao"`, `"chave_publica": "04`+strings.Repeat("ab", 64)+`", "descricao"`, 1)
	if _, err := LerRegistro(escrever(t, t.TempDir(), "registro.json", com)); err != nil {
		t.Fatal(err)
	}
}

func TestRegistroInvalido(t *testing.T) {
	casos := map[string]string{
		"schema errado":   strings.Replace(registroDourado, "registro.v1", "registro.v9", 1),
		"dev_id curto":    strings.Replace(registroDourado, "0011223344556677", "0011", 1),
		"perfil inválido": strings.Replace(registroDourado, `"hmac"`, `"rsa"`, 1),
		"sem chave auth":  strings.Replace(registroDourado, `"kms_chave_auth": "0011223344556677"`, `"kms_chave_auth": ""`, 1),
		"campo extra":     strings.Replace(registroDourado, `"perfil"`, `"segredo": "x", "perfil"`, 1),
		"json quebrado":   "{",
	}
	for nome, c := range casos {
		if _, err := LerRegistro(escrever(t, t.TempDir(), "registro.json", c)); err == nil {
			t.Errorf("%s: deveria falhar", nome)
		}
	}
}

func TestArquivosRegistro(t *testing.T) {
	dir := t.TempDir()
	a := escrever(t, dir, "0011223344556677/registro.json", registroDourado)
	b := escrever(t, dir, "0011223344556678/registro.json", strings.ReplaceAll(registroDourado, "0011223344556677", "0011223344556678"))
	escrever(t, dir, "0011223344556678/dongle.json", "{}")
	got, err := ArquivosRegistro(dir)
	if err != nil || len(got) != 2 || got[0] != a || got[1] != b {
		t.Fatalf("%v %v", err, got)
	}
	if got, _ := ArquivosRegistro(a); len(got) != 1 || got[0] != a {
		t.Fatalf("%v", got)
	}
	if _, err := ArquivosRegistro(filepath.Join(dir, "nao-existe")); err == nil {
		t.Fatal("caminho inexistente")
	}
}
