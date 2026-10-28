package disjuntor

import (
	"testing"
	"time"
)

func TestDisjuntor(t *testing.T) {
	agora := time.Unix(1000, 0)
	d := Novo(5, 10*time.Second)
	d.agora = func() time.Time { return agora }
	for i := 0; i < 5; i++ {
		if !d.Permitir() {
			t.Fatal("deveria permitir")
		}
		d.Falha()
	}
	if d.Permitir() || !d.Aberto() {
		t.Fatal("deveria estar aberto")
	}
	agora = agora.Add(11 * time.Second)
	if !d.Permitir() {
		t.Fatal("sonda deveria passar")
	}
	if d.Permitir() {
		t.Fatal("so uma sonda por vez")
	}
	d.Sucesso()
	if !d.Permitir() || d.Aberto() {
		t.Fatal("deveria fechar")
	}
}
