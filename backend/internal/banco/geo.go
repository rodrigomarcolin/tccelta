package banco

import (
	"encoding/binary"
	"math"
)

// PontoZ é um ponto geográfico (graus, metros).
type PontoZ struct{ LonGraus, LatGraus, AltM float64 }

// EWKB devolve a codificação binária EWKB (little-endian, PointZ com SRID 4326: 1+4+4+24 = 33 B),
// que é o formato binário do tipo geography do PostGIS.
func (p PontoZ) EWKB() []byte {
	b := make([]byte, 33)
	b[0] = 1                                         // little-endian
	binary.LittleEndian.PutUint32(b[1:], 0xA0000001) // PointZ | flag SRID
	binary.LittleEndian.PutUint32(b[5:], 4326)
	binary.LittleEndian.PutUint64(b[9:], math.Float64bits(p.LonGraus))
	binary.LittleEndian.PutUint64(b[17:], math.Float64bits(p.LatGraus))
	binary.LittleEndian.PutUint64(b[25:], math.Float64bits(p.AltM))
	return b
}
