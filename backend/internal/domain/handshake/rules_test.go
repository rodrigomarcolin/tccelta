package handshake

import (
	"testing"
	"time"
)

func TestApplyTail(t *testing.T) {
	usecases := []struct {
		name                string
		ack, head, tail, hd uint32
		want                uint32
		loss                *Range
		err                 bool
	}{
		{"sem descarte", 100, 120, 90, 130, 100, nil, false},
		{"tail = ack+1", 100, 120, 101, 130, 100, nil, false},
		{"overflow", 100, 120, 111, 130, 110, &Range{101, 110}, false},
		{"flash vazia", 130, 130, 131, 130, 130, nil, false},
		{"overflow total", 100, 120, 131, 130, 130, &Range{101, 130}, false},
		{"tail > head+1", 100, 120, 132, 130, 0, nil, true},
		{"head regrediu", 100, 120, 101, 119, 0, nil, true},
		{"dongle novo", 0, 0, 1, 0, 0, nil, false},
		{"dongle novo com overflow", 0, 0, 50, 80, 49, &Range{1, 49}, false},
	}
	for _, c := range usecases {
		ack, loss, err := ApplyTail(State{ContiguousSeq: c.ack, HeadSeq: c.head}, c.tail, c.hd)
		if (err != nil) != c.err {
			t.Errorf("%s: erro %v", c.name, err)
			continue
		}
		if c.err {
			continue
		}
		if ack != c.want || (loss == nil) != (c.loss == nil) || (loss != nil && *loss != *c.loss) {
			t.Errorf("%s: ack=%d perda=%v", c.name, ack, loss)
		}
	}
}

func TestAnchor(t *testing.T) {
	s1 := time.Unix(100, 0)
	max := 2 * time.Second
	if ok, _ := EvaluateAnchor(s1, s1, max); !ok {
		t.Error("rtt 0")
	}
	if ok, _ := EvaluateAnchor(s1, s1.Add(2*time.Second), max); !ok {
		t.Error("rtt 2s")
	}
	if ok, _ := EvaluateAnchor(s1, s1.Add(2*time.Second+time.Microsecond), max); ok {
		t.Error("rtt 2s+1us")
	}
	if ok, al := EvaluateAnchor(s1, s1.Add(-5*time.Millisecond), max); ok || !al {
		t.Error("rtt negativo")
	}
}

func TestGeneration(t *testing.T) {
	usecases := []struct {
		g, u, boot, want uint16
	}{{0, 7, 7, 0}, {0, 7, 9, 0}, {0, 7, 3, 1}, {2, 65535, 1, 3}, {0, 0, 1, 0}, {4, 9, 0, 4}}
	for _, c := range usecases {
		if got := HandshakeGeneration(State{BootGeneration: c.g, LastBootID: c.u}, c.boot); got != c.want {
			t.Errorf("%+v -> %d", c, got)
		}
	}
}

func TestRounding(t *testing.T) {
	x := time.Unix(10, 1500)
	if TruncateMicro(x).Nanosecond() != 1000 || MicroCeiling(x).Nanosecond() != 2000 {
		t.Fatal(TruncateMicro(x), MicroCeiling(x))
	}
	y := time.Unix(10, 2000)
	if MicroCeiling(y) != y {
		t.Fatal()
	}
}
