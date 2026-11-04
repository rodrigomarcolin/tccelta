package clientauth

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestValidID(t *testing.T) {
	for id, want := range map[string]bool{
		"next-feira": true, "abc": true, "a-1": true, "0ab": true, strings.Repeat("a", 64): true,
		"ab": false, "": false, "Abc": false, "-abc": false, "a_b": false, "a/b": false, "..": false, "a b": false,
		strings.Repeat("a", 65): false, "ábc": false,
	} {
		if ValidID(id) != want {
			t.Errorf("ValidID(%q) = %v", id, !want)
		}
	}
}

func TestValidSecret(t *testing.T) {
	for _, c := range []struct {
		s    string
		want bool
	}{{"", false}, {"a", true}, {strings.Repeat("x", 43), true}, {strings.Repeat("x", 128), true}, {strings.Repeat("x", 129), false}} {
		if ValidSecret(c.s) != c.want {
			t.Errorf("ValidSecret(%d caracteres) = %v", len(c.s), !c.want)
		}
	}
}

// A tabela da spec: com T=5, falhas 1 a 5 sem bloqueio; 6ª, 30 s; 7ª, 60 s; 8ª, 120 s; 9ª, 240 s; 10ª, 480 s; 11ª em diante, 15 min.
func TestPolicyBlockProgression(t *testing.T) {
	p := Policy{Tolerance: 5, Initial: 30 * time.Second, Factor: 2, Cap: 15 * time.Minute, Forget: 30 * time.Minute}
	want := []time.Duration{0, 0, 0, 0, 0, 30 * time.Second, 60 * time.Second, 120 * time.Second, 240 * time.Second, 480 * time.Second,
		15 * time.Minute, 15 * time.Minute}
	for i, w := range want {
		if got := p.Block(i + 1); got != w {
			t.Errorf("falha %d: %v, esperava %v", i+1, got, w)
		}
	}
	if got := p.Block(1_000_000); got != 15*time.Minute {
		t.Errorf("muitas falhas deve estourar no teto sem overflow: %v", got)
	}
	if got := p.Block(0); got != 0 {
		t.Errorf("n=0: %v", got)
	}
}

func TestPolicyBlockFactorOne(t *testing.T) {
	p := Policy{Tolerance: 1, Initial: time.Second, Factor: 1, Cap: time.Minute, Forget: time.Hour}
	if p.Block(1) != 0 || p.Block(2) != time.Second || p.Block(50) != time.Second {
		t.Fatalf("%v %v %v", p.Block(1), p.Block(2), p.Block(50))
	}
}

func TestPolicyValidate(t *testing.T) {
	ok := Policy{Tolerance: 5, Initial: 30 * time.Second, Factor: 2, Cap: 15 * time.Minute, Forget: 30 * time.Minute}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, mut := range map[string]func(*Policy){
		"tolerância 0":   func(p *Policy) { p.Tolerance = 0 },
		"fator < 1":      func(p *Policy) { p.Factor = 0.5 },
		"teto < inicial": func(p *Policy) { p.Cap = time.Second },
		"inicial 0":      func(p *Policy) { p.Initial = 0 },
		"esquecer 0":     func(p *Policy) { p.Forget = 0 },
	} {
		p := ok
		mut(&p)
		if p.Validate() == nil {
			t.Errorf("%s deveria ser recusada", name)
		}
	}
}

func TestBlockedError(t *testing.T) {
	var err error = BlockedError{RetryAfter: 90 * time.Second}
	if !errors.Is(err, ErrBlocked) || errors.Is(err, ErrInvalidClient) {
		t.Fatal("Is")
	}
	var be BlockedError
	if !errors.As(err, &be) || be.RetryAfter != 90*time.Second || !strings.Contains(err.Error(), "1m30s") {
		t.Fatalf("%v %v", be, err)
	}
}
