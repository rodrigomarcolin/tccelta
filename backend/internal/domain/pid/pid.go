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
	Nome, Unidade       string
	NBytes              int
	Fator, Deslocamento float64
	Min, Max            *float64
}

// Consulta é o que as regras precisam do catálogo.
type Consulta interface {
	Obter(pid uint8) (Def, bool)
}

// Catalogo é um mapa atômico de definições, seguro para uso concorrente.
type Catalogo struct {
	m atomic.Pointer[map[uint8]Def]
}

// Novo cria um catálogo vazio; chame Definir antes de usar.
func Novo() *Catalogo { return &Catalogo{} }

// DeMapa cria um catálogo fixo (testes).
func DeMapa(defs []Def) *Catalogo {
	c := &Catalogo{}
	c.Definir(defs)
	return c
}

// Definir substitui todas as definições.
func (c *Catalogo) Definir(defs []Def) {
	m := make(map[uint8]Def, len(defs))
	for _, d := range defs {
		m[d.PID] = d
	}
	c.m.Store(&m)
}

// Obter devolve a definição do PID.
func (c *Catalogo) Obter(pid uint8) (Def, bool) {
	m := c.m.Load()
	if m == nil {
		return Def{}, false
	}
	d, ok := (*m)[pid]
	return d, ok
}

// Carregado diz se há catálogo (readiness).
func (c *Catalogo) Carregado() bool { m := c.m.Load(); return m != nil && len(*m) > 0 }

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
