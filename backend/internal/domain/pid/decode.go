package pid

import (
	"errors"

	"tccelta/backend/internal/domain/protocol"
)

// ErrUnknown: PID fora do catálogo.
var ErrUnknown = errors.New("pid_desconhecido")

// Reading é uma linha de silver.reading.
type Reading struct {
	Generation, BootID int32
	TMs                int64
	PID                int16
	RawValue           int32
	Value              *float32
	BlockSeq           int64
}

// Position é uma linha de silver.position.
type Position struct {
	Generation, BootID int32
	TMs                int64
	Geom               PointZ
	BlockSeq           int64
}

// Decode aplica o catálogo: bruto = A (1 byte) ou 256*A + B (2 bytes); valor = bruto*fator + deslocamento;
// fora de [min, max] => Value = nil. t0 é o t0_ms já desembrulhado.
func Decode(generation uint16, hdr protocol.BlockHeader, c protocol.Content, t0 int64, cat Query) ([]Reading, []Position, error) {
	ls := make([]Reading, 0, len(c.PIDs))
	for _, p := range c.PIDs {
		d, ok := cat.Get(p.PID)
		if !ok {
			return nil, nil, ErrUnknown
		}
		raw := int32(p.A)
		if d.NBytes == 2 {
			raw = int32(p.A)<<8 | int32(p.B)
		}
		v := float64(raw)*d.Factor + d.ValueOffset
		var pv *float32
		if (d.Min == nil || v >= *d.Min-1e-9) && (d.Max == nil || v <= *d.Max+1e-9) {
			f := float32(v)
			pv = &f
		}
		ls = append(ls, Reading{Generation: int32(generation), BootID: int32(hdr.BootID), TMs: t0 + int64(p.Dt),
			PID: int16(p.PID), RawValue: raw, Value: pv, BlockSeq: int64(hdr.Seq)})
	}
	ps := make([]Position, 0, len(c.GPS))
	for _, g := range c.GPS {
		ps = append(ps, Position{Generation: int32(generation), BootID: int32(hdr.BootID), TMs: t0 + int64(g.Dt),
			Geom:     PointZ{LonDegrees: float64(g.LonE7) / 1e7, LatDegrees: float64(g.LatE7) / 1e7, AltM: float64(g.AltMm) / 1000},
			BlockSeq: int64(hdr.Seq)})
	}
	return ls, ps, nil
}
