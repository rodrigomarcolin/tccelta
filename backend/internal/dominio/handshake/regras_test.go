package handshake

import (
	"testing"
	"time"
)

func TestAplicarTail(t *testing.T) {
	casos := []struct {
		nome                string
		ack, head, tail, hd uint32
		esp                 uint32
		perda               *Faixa
		erro                bool
	}{
		{"sem descarte", 100, 120, 90, 130, 100, nil, false},
		{"tail = ack+1", 100, 120, 101, 130, 100, nil, false},
		{"overflow", 100, 120, 111, 130, 110, &Faixa{101, 110}, false},
		{"flash vazia", 130, 130, 131, 130, 130, nil, false},
		{"overflow total", 100, 120, 131, 130, 130, &Faixa{101, 130}, false},
		{"tail > head+1", 100, 120, 132, 130, 0, nil, true},
		{"head regrediu", 100, 120, 101, 119, 0, nil, true},
		{"dongle novo", 0, 0, 1, 0, 0, nil, false},
		{"dongle novo com overflow", 0, 0, 50, 80, 49, &Faixa{1, 49}, false},
	}
	for _, c := range casos {
		ack, perda, err := AplicarTail(Estado{SeqContiguo: c.ack, HeadSeq: c.head}, c.tail, c.hd)
		if (err != nil) != c.erro {
			t.Errorf("%s: erro %v", c.nome, err)
			continue
		}
		if c.erro {
			continue
		}
		if ack != c.esp || (perda == nil) != (c.perda == nil) || (perda != nil && *perda != *c.perda) {
			t.Errorf("%s: ack=%d perda=%v", c.nome, ack, perda)
		}
	}
}

func TestAncora(t *testing.T) {
	s1 := time.Unix(100, 0)
	max := 2 * time.Second
	if ok, _ := AvaliarAncora(s1, s1, max); !ok {
		t.Error("rtt 0")
	}
	if ok, _ := AvaliarAncora(s1, s1.Add(2*time.Second), max); !ok {
		t.Error("rtt 2s")
	}
	if ok, _ := AvaliarAncora(s1, s1.Add(2*time.Second+time.Microsecond), max); ok {
		t.Error("rtt 2s+1us")
	}
	if ok, al := AvaliarAncora(s1, s1.Add(-5*time.Millisecond), max); ok || !al {
		t.Error("rtt negativo")
	}
}

func TestGeracao(t *testing.T) {
	casos := []struct {
		g, u, boot, esp uint16
	}{{0, 7, 7, 0}, {0, 7, 9, 0}, {0, 7, 3, 1}, {2, 65535, 1, 3}, {0, 0, 1, 0}, {4, 9, 0, 4}}
	for _, c := range casos {
		if got := GeracaoDoHandshake(Estado{GeracaoBoot: c.g, UltimoBootID: c.u}, c.boot); got != c.esp {
			t.Errorf("%+v -> %d", c, got)
		}
	}
}

func TestArredondamento(t *testing.T) {
	x := time.Unix(10, 1500)
	if TruncarMicro(x).Nanosecond() != 1000 || TetoMicro(x).Nanosecond() != 2000 {
		t.Fatal(TruncarMicro(x), TetoMicro(x))
	}
	y := time.Unix(10, 2000)
	if TetoMicro(y) != y {
		t.Fatal()
	}
}
