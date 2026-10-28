package breaker

import (
	"testing"
	"time"
)

func TestBreaker(t *testing.T) {
	now := time.Unix(1000, 0)
	d := New(5, 10*time.Second)
	d.now = func() time.Time { return now }
	for i := 0; i < 5; i++ {
		if !d.Allow() {
			t.Fatal("deveria permitir")
		}
		d.Failure()
	}
	if d.Allow() || !d.Open() {
		t.Fatal("deveria estar aberto")
	}
	now = now.Add(11 * time.Second)
	if !d.Allow() {
		t.Fatal("sonda deveria passar")
	}
	if d.Allow() {
		t.Fatal("so uma sonda por vez")
	}
	d.Success()
	if !d.Allow() || d.Open() {
		t.Fatal("deveria fechar")
	}
}
