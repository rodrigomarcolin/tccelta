package pid

import (
	"errors"

	"tccelta/backend/internal/dominio/protocolo"
)

// ErrDesconhecido: PID fora do catálogo.
var ErrDesconhecido = errors.New("pid_desconhecido")

// Leitura é uma linha de silver.leitura.
type Leitura struct {
	Geracao, BootID int32
	TMs             int64
	PID             int16
	ValorBruto      int32
	Valor           *float32
	SeqBloco        int64
}

// Posicao é uma linha de silver.posicao.
type Posicao struct {
	Geracao, BootID int32
	TMs             int64
	Geom            PontoZ
	SeqBloco        int64
}

// Decodificar aplica o catálogo: bruto = A (1 byte) ou 256*A + B (2 bytes); valor = bruto*fator + deslocamento;
// fora de [min, max] => Valor = nil. t0 é o t0_ms já desembrulhado.
func Decodificar(geracao uint16, cab protocolo.CabecalhoBloco, c protocolo.Conteudo, t0 int64, cat Consulta) ([]Leitura, []Posicao, error) {
	ls := make([]Leitura, 0, len(c.PIDs))
	for _, p := range c.PIDs {
		d, ok := cat.Obter(p.PID)
		if !ok {
			return nil, nil, ErrDesconhecido
		}
		bruto := int32(p.A)
		if d.NBytes == 2 {
			bruto = int32(p.A)<<8 | int32(p.B)
		}
		v := float64(bruto)*d.Fator + d.Deslocamento
		var pv *float32
		if (d.Min == nil || v >= *d.Min-1e-9) && (d.Max == nil || v <= *d.Max+1e-9) {
			f := float32(v)
			pv = &f
		}
		ls = append(ls, Leitura{Geracao: int32(geracao), BootID: int32(cab.BootID), TMs: t0 + int64(p.Dt),
			PID: int16(p.PID), ValorBruto: bruto, Valor: pv, SeqBloco: int64(cab.Seq)})
	}
	ps := make([]Posicao, 0, len(c.GPS))
	for _, g := range c.GPS {
		ps = append(ps, Posicao{Geracao: int32(geracao), BootID: int32(cab.BootID), TMs: t0 + int64(g.Dt),
			Geom:     PontoZ{LonGraus: float64(g.LonE7) / 1e7, LatGraus: float64(g.LatE7) / 1e7, AltM: float64(g.AltMm) / 1000},
			SeqBloco: int64(cab.Seq)})
	}
	return ls, ps, nil
}
