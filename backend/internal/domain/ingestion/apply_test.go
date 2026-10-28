package ingestion

import (
	"testing"

	"tccelta/backend/internal/domain/pid"
	"tccelta/backend/internal/domain/protocol"
)

func f(v float64) *float64 { return &v }

var cat = pid.FromMap([]pid.Def{
	{PID: 4, NBytes: 1, Factor: 100.0 / 255, Min: f(0), Max: f(100)},
	{PID: 5, NBytes: 1, Factor: 1, ValueOffset: -40, Min: f(-40), Max: f(215)},
	{PID: 12, NBytes: 2, Factor: 0.25, Min: f(0), Max: f(16383.75)},
	{PID: 13, NBytes: 1, Factor: 1, Min: f(0), Max: f(255)},
})

type bo struct {
	seq           uint32
	boot          uint16
	tag           bool
	pid           uint8 // 0 => 12
	marker        protocol.RecordType
	from, to      uint32
	withoutVerify bool
	data          []byte
}

func mk(x bo) BlockIn {
	var d []byte
	switch {
	case x.data != nil:
		d = x.data
	case x.marker != 0:
		d = protocol.BuildMarker(x.seq, x.boot, 0, x.marker, x.from, x.to)
	default:
		m := protocol.NewBlockBuilder(x.seq, x.boot, 1000)
		p := x.pid
		if p == 0 {
			p = 12
		}
		m.PID(p, 0, 1, 2)
		d = m.Data()
	}
	b, err := protocol.DecodeBlock(append(append([]byte{}, d...), make([]byte, 32)...), protocol.ProfileHMAC)
	if err != nil {
		panic(err)
	}
	return BlockIn{B: b, Valid: x.tag, Verified: !x.withoutVerify}
}

func batch(xs ...bo) []BlockIn {
	var out []BlockIn
	for _, x := range xs {
		out = append(out, mk(x))
	}
	return out
}

func seqs(a, b uint32) []bo {
	var o []bo
	for s := a; s <= b; s++ {
		o = append(o, bo{seq: s, boot: 1, tag: true})
	}
	return o
}

func TestApply(t *testing.T) {
	usecases := []struct {
		name   string
		s      uint32
		g, u   uint16
		batch  []bo
		ack    uint32
		reason Reason
		dups   int
		gaps   []GapOut
		seen   uint32
		gFinal uint16
	}{
		{name: "1", s: 10, batch: seqs(11, 20), ack: 20, seen: 20},
		{name: "2", s: 10, batch: seqs(5, 15), ack: 15, dups: 6, seen: 15},
		{name: "3", s: 10, batch: []bo{{seq: 11, boot: 1, tag: true}, {seq: 12, boot: 1, tag: true}, {seq: 14, boot: 1, tag: true}}, ack: 12, reason: ReasonJump, seen: 14},
		{name: "4", s: 10, batch: []bo{{seq: 11, boot: 1, tag: true}, {seq: 12, boot: 1}, {seq: 13, boot: 1, tag: true}}, ack: 11, reason: ReasonInvalidTag, seen: 13},
		{name: "5", s: 10, batch: []bo{{seq: 11, boot: 1}}, ack: 10, reason: ReasonInvalidTag},
		{name: "6", s: 10, batch: []bo{{seq: 11, boot: 1, tag: true}, {seq: 64, boot: 2, tag: true, marker: protocol.RegBoot, from: 12, to: 63}, {seq: 65, boot: 2, tag: true}},
			ack: 65, seen: 65, gaps: []GapOut{{From: 12, To: 63, BlockSeq: 64, Cause: "BOOT"}}},
		{name: "7", s: 10, batch: []bo{{seq: 11, boot: 2, tag: true, marker: protocol.RegBoot, from: 11, to: 10}}, ack: 11, seen: 11},
		{name: "8", s: 10, batch: []bo{{seq: 64, boot: 2, tag: true, marker: protocol.RegBoot, from: 13, to: 63}}, ack: 10, reason: ReasonInvalidMarker, seen: 64},
		{name: "9", s: 10, batch: []bo{{seq: 11, tag: true, marker: protocol.RegLoss, from: 11, to: 15}, {seq: 16, boot: 1, tag: true}},
			ack: 16, seen: 16, gaps: []GapOut{{From: 11, To: 15, BlockSeq: 11, Cause: "LOSS"}}},
		{name: "10", s: 10, batch: []bo{{seq: 11, tag: true, marker: protocol.RegLoss, from: 11, to: 15}, {seq: 13, boot: 1, tag: true}},
			ack: 15, dups: 1, seen: 13, gaps: []GapOut{{From: 11, To: 15, BlockSeq: 11, Cause: "LOSS"}}},
		{name: "11", s: 10, batch: []bo{{seq: 11, boot: 1, tag: true, pid: 0x99}}, ack: 10, reason: ReasonUnknownPID, seen: 11},
		{name: "13", s: 10, batch: []bo{{seq: 11, boot: 1, tag: true}, {seq: 11, boot: 1, tag: true}, {seq: 12, boot: 1, tag: true}}, ack: 12, dups: 1, seen: 12},
		{name: "16", s: 63, batch: []bo{{seq: 64, boot: 3, tag: true, marker: protocol.RegBoot, from: 31, to: 63}, {seq: 65, boot: 3, tag: true}}, ack: 65, seen: 65},
		{name: "17", s: 30, batch: []bo{{seq: 64, boot: 3, tag: true, marker: protocol.RegBoot, from: 20, to: 63}}, ack: 64, seen: 64,
			gaps: []GapOut{{From: 31, To: 63, BlockSeq: 64, Cause: "BOOT"}}},
		{name: "18", s: 30, batch: []bo{{seq: 64, tag: true, marker: protocol.RegLoss, from: 31, to: 64}}, ack: 64, seen: 64,
			gaps: []GapOut{{From: 31, To: 64, BlockSeq: 64, Cause: "LOSS"}}},
		{name: "19", s: 30, batch: []bo{{seq: 31, tag: true, marker: protocol.RegLoss, from: 32, to: 40}}, ack: 30, reason: ReasonInvalidMarker, seen: 31},
		{name: "20", s: 30, batch: []bo{{seq: 31, boot: 3, tag: true, marker: protocol.RegBoot, from: 31, to: 30}}, ack: 31, seen: 31},
		{name: "21", s: 30, batch: []bo{{seq: 31, tag: true, marker: protocol.RegLoss, from: 31, to: 63}, {seq: 64, tag: true, marker: protocol.RegLoss, from: 64, to: 64}, {seq: 65, boot: 3, tag: true}},
			ack: 65, seen: 65, gaps: []GapOut{{From: 31, To: 63, BlockSeq: 31, Cause: "LOSS"}, {From: 64, To: 64, BlockSeq: 64, Cause: "LOSS"}}},
		{name: "22", s: 10, batch: []bo{{seq: 11, boot: 1, tag: true}, {seq: 13, boot: 1, tag: true}, {seq: 14, boot: 1}}, ack: 11, reason: ReasonJump, seen: 13},
		{name: "23", s: 10, g: 0, u: 65535, batch: []bo{{seq: 11, boot: 1, tag: true, marker: protocol.RegBoot, from: 11, to: 10}, {seq: 12, boot: 1, tag: true}},
			ack: 12, seen: 12, gFinal: 1},
		{name: "24", s: 10, g: 0, u: 7, batch: []bo{{seq: 11, boot: 7, tag: true}, {seq: 12, tag: true, marker: protocol.RegLoss, from: 12, to: 14}, {seq: 15, boot: 7, tag: true}, {seq: 5, boot: 3, tag: true}},
			ack: 15, dups: 1, seen: 15, gaps: []GapOut{{From: 12, To: 14, BlockSeq: 12, Cause: "LOSS"}}},
	}
	for _, c := range usecases {
		r := Apply(batch(c.batch...), StateIn{ContiguousSeq: c.s, G: c.g, U: c.u}, cat)
		if r.NewAck != c.ack || r.Reason != c.reason || r.Duplicates != c.dups || r.MaxSeen != c.seen || r.G != c.gFinal {
			t.Errorf("%s: ack=%d motivo=%q dups=%d visto=%d g=%d", c.name, r.NewAck, r.Reason, r.Duplicates, r.MaxSeen, r.G)
		}
		if len(r.Gaps) != len(c.gaps) {
			t.Errorf("%s: lacunas %v", c.name, r.Gaps)
			continue
		}
		for i := range c.gaps {
			if r.Gaps[i] != c.gaps[i] {
				t.Errorf("%s: lacuna %v != %v", c.name, r.Gaps[i], c.gaps[i])
			}
		}
	}
}

func TestMalformed(t *testing.T) {
	// marcador + PID no mesmo bloco
	m := protocol.NewBlockBuilder(11, 1, 0)
	m.PID(4, 0, 1, 0)
	d := append(append([]byte{}, m.Data()...), protocol.BuildMarker(11, 1, 0, protocol.RegBoot, 11, 10)[12:]...)
	d[10], d[11] = byte(len(d)), byte(len(d)>>8)
	r := Apply(batch(bo{seq: 11, boot: 1, tag: true, data: d}), StateIn{ContiguousSeq: 10}, cat)
	if r.Reason != ReasonMalformedRecord || r.NewAck != 10 {
		t.Fatalf("%+v", r)
	}
	if ActionOf(r.Reason) != ActionWait || ActionOf(ReasonJump) != ActionReread || ActionOf("") != ActionNone {
		t.Fatal("acoes")
	}
}

func TestAssignGeneration(t *testing.T) {
	usecases := []struct {
		g, u, b, wantG, wantU uint16
		av                    bool
	}{{0, 7, 9, 0, 9, false}, {0, 7, 7, 0, 7, false}, {0, 7, 3, 1, 3, true}, {0, 65535, 1, 1, 1, true}, {2, 5, 0, 2, 5, false}}
	for _, c := range usecases {
		g, u, av := assignGeneration(c.g, c.u, c.b)
		if g != c.wantG || u != c.wantU || av != c.av {
			t.Errorf("%+v -> %d %d %v", c, g, u, av)
		}
	}
}
