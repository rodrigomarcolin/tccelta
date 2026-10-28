package pid

import (
	"math"
	"testing"

	"tccelta/backend/internal/domain/protocol"
)

func f(v float64) *float64 { return &v }

var cat = FromMap([]Def{
	{PID: 4, NBytes: 1, Factor: 100.0 / 255, Min: f(0), Max: f(100)},
	{PID: 5, NBytes: 1, Factor: 1, ValueOffset: -40, Min: f(-40), Max: f(215)},
	{PID: 12, NBytes: 2, Factor: 0.25, Min: f(0), Max: f(16383.75)},
	{PID: 13, NBytes: 1, Factor: 1, Min: f(0), Max: f(255)},
})

func TestDecode(t *testing.T) {
	m := protocol.NewBlockBuilder(1, 3, 600000)
	m.PID(12, 0, 0x17, 0x70)
	m.PID(5, 10, 0x28, 0)
	m.PID(13, 20, 0xFF, 0)
	m.PID(5, 30, 0xFF, 0) // 255-40 = 215 (dentro);
	m.GPS(65535, -235505200, -466333100, 760000)
	d := m.Data()
	c, err := protocol.ReadRecords(d)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := protocol.DecodeBlock(append(append([]byte{}, d...), make([]byte, 32)...), protocol.ProfileHMAC)
	ls, ps, err := Decode(0, b.Hdr, c, 600000, cat)
	if err != nil {
		t.Fatal(err)
	}
	if ls[0].RawValue != 6000 || *ls[0].Value != 1500 || *ls[1].Value != 0 || ls[2].RawValue != 255 || ls[3].TMs != 600030 {
		t.Fatalf("%+v", ls)
	}
	if ps[0].TMs != 600000+65535 || math.Abs(ps[0].Geom.LatDegrees+23.55052) > 1e-9 || ps[0].Geom.AltM != 760 {
		t.Fatalf("%+v", ps)
	}
	// fora da faixa
	c2 := protocol.Content{PIDs: []protocol.PIDRecord{{PID: 4, A: 255}}}
	cat2 := FromMap([]Def{{PID: 4, NBytes: 1, Factor: 1, Min: f(0), Max: f(100)}})
	l2, _, _ := Decode(0, b.Hdr, c2, 0, cat2)
	if l2[0].Value != nil {
		t.Fatal("deveria ser nulo")
	}
	if _, _, err := Decode(0, b.Hdr, protocol.Content{PIDs: []protocol.PIDRecord{{PID: 0x99}}}, 0, cat); err != ErrUnknown {
		t.Fatal(err)
	}
}
