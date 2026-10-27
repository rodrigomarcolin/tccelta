package kms

import (
	"testing"
	"time"
)

func TestDisjuntor(t *testing.T) {
	agora := time.Unix(1000, 0)
	d := novoDisjuntor(5, 10*time.Second)
	d.agora = func() time.Time { return agora }
	for i := 0; i < 5; i++ {
		if !d.permitir() {
			t.Fatal("deveria permitir")
		}
		d.falha()
	}
	if d.permitir() || !d.aberto() {
		t.Fatal("deveria estar aberto")
	}
	agora = agora.Add(11 * time.Second)
	if !d.permitir() {
		t.Fatal("sonda deveria passar")
	}
	if d.permitir() {
		t.Fatal("so uma sonda por vez")
	}
	d.sucesso()
	if !d.permitir() || d.aberto() {
		t.Fatal("deveria fechar")
	}
}
