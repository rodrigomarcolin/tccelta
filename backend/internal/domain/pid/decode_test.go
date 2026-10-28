package pid

import (
	"math"
	"testing"

	"tccelta/backend/internal/dominio/protocolo"
)

func f(v float64) *float64 { return &v }

var cat = DeMapa([]Def{
	{PID: 4, NBytes: 1, Fator: 100.0 / 255, Min: f(0), Max: f(100)},
	{PID: 5, NBytes: 1, Fator: 1, Deslocamento: -40, Min: f(-40), Max: f(215)},
	{PID: 12, NBytes: 2, Fator: 0.25, Min: f(0), Max: f(16383.75)},
	{PID: 13, NBytes: 1, Fator: 1, Min: f(0), Max: f(255)},
})

func TestDecodificar(t *testing.T) {
	m := protocolo.NovoMontadorBloco(1, 3, 600000)
	m.PID(12, 0, 0x17, 0x70)
	m.PID(5, 10, 0x28, 0)
	m.PID(13, 20, 0xFF, 0)
	m.PID(5, 30, 0xFF, 0) // 255-40 = 215 (dentro);
	m.GPS(65535, -235505200, -466333100, 760000)
	d := m.Dados()
	c, err := protocolo.LerRegistros(d)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := protocolo.DecodificarBloco(append(append([]byte{}, d...), make([]byte, 32)...), protocolo.PerfilHMAC)
	ls, ps, err := Decodificar(0, b.Cab, c, 600000, cat)
	if err != nil {
		t.Fatal(err)
	}
	if ls[0].ValorBruto != 6000 || *ls[0].Valor != 1500 || *ls[1].Valor != 0 || ls[2].ValorBruto != 255 || ls[3].TMs != 600030 {
		t.Fatalf("%+v", ls)
	}
	if ps[0].TMs != 600000+65535 || math.Abs(ps[0].Geom.LatGraus+23.55052) > 1e-9 || ps[0].Geom.AltM != 760 {
		t.Fatalf("%+v", ps)
	}
	// fora da faixa
	c2 := protocolo.Conteudo{PIDs: []protocolo.RegistroPID{{PID: 4, A: 255}}}
	cat2 := DeMapa([]Def{{PID: 4, NBytes: 1, Fator: 1, Min: f(0), Max: f(100)}})
	l2, _, _ := Decodificar(0, b.Cab, c2, 0, cat2)
	if l2[0].Valor != nil {
		t.Fatal("deveria ser nulo")
	}
	if _, _, err := Decodificar(0, b.Cab, protocolo.Conteudo{PIDs: []protocolo.RegistroPID{{PID: 0x99}}}, 0, cat); err != ErrDesconhecido {
		t.Fatal(err)
	}
}
