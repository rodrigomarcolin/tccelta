package config

import (
	"os"
	"path/filepath"
	"testing"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestPadroes(t *testing.T) {
	c, err := CarregarDe(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if c.Protocolo.LoteMaxBlocos != 32 || c.JWT.Validade.Hours() != 1 || c.Ambiente != "dev" {
		t.Fatalf("%+v", c)
	}
}

func TestSegredoArquivo(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "s")
	_ = os.WriteFile(p, []byte("abc\n"), 0o600)
	c, err := CarregarDe(env(map[string]string{"BANCO_SENHA_ARQUIVO": p}))
	if err != nil || c.Banco.Senha != "abc" {
		t.Fatalf("%v %q", err, c.Banco.Senha)
	}
	if _, err := CarregarDe(env(map[string]string{"BANCO_SENHA_ARQUIVO": filepath.Join(d, "nao")})); err == nil {
		t.Fatal("esperava erro")
	}
}

func TestProducaoRecusaNone(t *testing.T) {
	if _, err := CarregarDe(env(map[string]string{"AMBIENTE": "producao", "PERMITIR_PERFIL_NONE": "true"})); err == nil {
		t.Fatal("esperava erro")
	}
	if _, err := CarregarDe(env(map[string]string{"AMBIENTE": "xx"})); err == nil {
		t.Fatal("ambiente inválido")
	}
	if _, err := CarregarDe(env(map[string]string{"JWT_VALIDADE": "abc"})); err == nil {
		t.Fatal("duração inválida")
	}
}
