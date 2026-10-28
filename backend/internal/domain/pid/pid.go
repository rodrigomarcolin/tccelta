// Package pid guarda o catálogo de PIDs e decodifica os registros de um bloco em leituras e posições.
package pid

import (
	"encoding/binary"
	"math"
	"sync/atomic"
)

// Def é a definição de um PID.
type Def struct {
	PID                 uint8
	Name, Unit          string
	NBytes              int
	Factor, ValueOffset float64
	Min, Max            *float64
}

// Query é o que as regras precisam do catálogo.
type Query interface {
	Get(pid uint8) (Def, bool)
}

// Catalog é um mapa atômico de definições, seguro para uso concorrente.
type Catalog struct {
	m atomic.Pointer[map[uint8]Def]
}

// New cria um catálogo vazio; chame Set antes de usar.
func New() *Catalog { return &Catalog{} }

// FromMap cria um catálogo fixo (testes).
func FromMap(defs []Def) *Catalog {
	c := &Catalog{}
	c.Set(defs)
	return c
}

// Set substitui todas as definições.
func (c *Catalog) Set(defs []Def) {
	m := make(map[uint8]Def, len(defs))
	for _, d := range defs {
		m[d.PID] = d
	}
	c.m.Store(&m)
}

// Get devolve a definição do PID.
func (c *Catalog) Get(pid uint8) (Def, bool) {
	m := c.m.Load()
	if m == nil {
		return Def{}, false
	}
	d, ok := (*m)[pid]
	return d, ok
}

// Loaded diz se há catálogo (readiness).
func (c *Catalog) Loaded() bool { m := c.m.Load(); return m != nil && len(*m) > 0 }

// PointZ é um ponto geográfico (graus, metros).
type PointZ struct{ LonDegrees, LatDegrees, AltM float64 }

// EWKB devolve a codificação binária EWKB (little-endian, PointZ com SRID 4326: 1+4+4+24 = 33 B),
// que é o formato binário do tipo geography do PostGIS.
func (p PointZ) EWKB() []byte {
	b := make([]byte, 33)
	b[0] = 1                                         // little-endian
	binary.LittleEndian.PutUint32(b[1:], 0xA0000001) // PointZ | flag SRID
	binary.LittleEndian.PutUint32(b[5:], 4326)
	binary.LittleEndian.PutUint64(b[9:], math.Float64bits(p.LonDegrees))
	binary.LittleEndian.PutUint64(b[17:], math.Float64bits(p.LatDegrees))
	binary.LittleEndian.PutUint64(b[25:], math.Float64bits(p.AltM))
	return b
}
