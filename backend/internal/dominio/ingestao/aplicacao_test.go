package ingestao

import (
	"testing"

	"tccelta/backend/internal/dominio/pid"
	"tccelta/backend/internal/dominio/protocolo"
)

func f(v float64) *float64 { return &v }

var cat = pid.DeMapa([]pid.Def{
	{PID: 4, NBytes: 1, Fator: 100.0 / 255, Min: f(0), Max: f(100)},
	{PID: 5, NBytes: 1, Fator: 1, Deslocamento: -40, Min: f(-40), Max: f(215)},
	{PID: 12, NBytes: 2, Fator: 0.25, Min: f(0), Max: f(16383.75)},
	{PID: 13, NBytes: 1, Fator: 1, Min: f(0), Max: f(255)},
})

type bo struct {
	seq          uint32
	boot         uint16
	tag          bool
	pid          uint8 // 0 => 12
	marc         protocolo.TipoRegistro
	from, to     uint32
	semVerificar bool
	dados        []byte
}

func mk(x bo) BlocoIn {
	var d []byte
	switch {
	case x.dados != nil:
		d = x.dados
	case x.marc != 0:
		d = protocolo.MontarMarcador(x.seq, x.boot, 0, x.marc, x.from, x.to)
	default:
		m := protocolo.NovoMontadorBloco(x.seq, x.boot, 1000)
		p := x.pid
		if p == 0 {
			p = 12
		}
		m.PID(p, 0, 1, 2)
		d = m.Dados()
	}
	b, err := protocolo.DecodificarBloco(append(append([]byte{}, d...), make([]byte, 32)...), protocolo.PerfilHMAC)
	if err != nil {
		panic(err)
	}
	return BlocoIn{B: b, Valido: x.tag, Verificado: !x.semVerificar}
}

func lote(xs ...bo) []BlocoIn {
	var out []BlocoIn
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

func TestAplicar(t *testing.T) {
	casos := []struct {
		nome   string
		s      uint32
		g, u   uint16
		lote   []bo
		ack    uint32
		motivo Motivo
		dups   int
		lacs   []LacunaOut
		visto  uint32
		gFinal uint16
	}{
		{nome: "1", s: 10, lote: seqs(11, 20), ack: 20, visto: 20},
		{nome: "2", s: 10, lote: seqs(5, 15), ack: 15, dups: 6, visto: 15},
		{nome: "3", s: 10, lote: []bo{{seq: 11, boot: 1, tag: true}, {seq: 12, boot: 1, tag: true}, {seq: 14, boot: 1, tag: true}}, ack: 12, motivo: MotivoSalto, visto: 14},
		{nome: "4", s: 10, lote: []bo{{seq: 11, boot: 1, tag: true}, {seq: 12, boot: 1}, {seq: 13, boot: 1, tag: true}}, ack: 11, motivo: MotivoTagInvalida, visto: 13},
		{nome: "5", s: 10, lote: []bo{{seq: 11, boot: 1}}, ack: 10, motivo: MotivoTagInvalida},
		{nome: "6", s: 10, lote: []bo{{seq: 11, boot: 1, tag: true}, {seq: 64, boot: 2, tag: true, marc: protocolo.RegBoot, from: 12, to: 63}, {seq: 65, boot: 2, tag: true}},
			ack: 65, visto: 65, lacs: []LacunaOut{{From: 12, To: 63, SeqBloco: 64, Causa: "BOOT"}}},
		{nome: "7", s: 10, lote: []bo{{seq: 11, boot: 2, tag: true, marc: protocolo.RegBoot, from: 11, to: 10}}, ack: 11, visto: 11},
		{nome: "8", s: 10, lote: []bo{{seq: 64, boot: 2, tag: true, marc: protocolo.RegBoot, from: 13, to: 63}}, ack: 10, motivo: MotivoMarcadorInvalido, visto: 64},
		{nome: "9", s: 10, lote: []bo{{seq: 11, tag: true, marc: protocolo.RegLoss, from: 11, to: 15}, {seq: 16, boot: 1, tag: true}},
			ack: 16, visto: 16, lacs: []LacunaOut{{From: 11, To: 15, SeqBloco: 11, Causa: "LOSS"}}},
		{nome: "10", s: 10, lote: []bo{{seq: 11, tag: true, marc: protocolo.RegLoss, from: 11, to: 15}, {seq: 13, boot: 1, tag: true}},
			ack: 15, dups: 1, visto: 13, lacs: []LacunaOut{{From: 11, To: 15, SeqBloco: 11, Causa: "LOSS"}}},
		{nome: "11", s: 10, lote: []bo{{seq: 11, boot: 1, tag: true, pid: 0x99}}, ack: 10, motivo: MotivoPIDDesconhecido, visto: 11},
		{nome: "13", s: 10, lote: []bo{{seq: 11, boot: 1, tag: true}, {seq: 11, boot: 1, tag: true}, {seq: 12, boot: 1, tag: true}}, ack: 12, dups: 1, visto: 12},
		{nome: "16", s: 63, lote: []bo{{seq: 64, boot: 3, tag: true, marc: protocolo.RegBoot, from: 31, to: 63}, {seq: 65, boot: 3, tag: true}}, ack: 65, visto: 65},
		{nome: "17", s: 30, lote: []bo{{seq: 64, boot: 3, tag: true, marc: protocolo.RegBoot, from: 20, to: 63}}, ack: 64, visto: 64,
			lacs: []LacunaOut{{From: 31, To: 63, SeqBloco: 64, Causa: "BOOT"}}},
		{nome: "18", s: 30, lote: []bo{{seq: 64, tag: true, marc: protocolo.RegLoss, from: 31, to: 64}}, ack: 64, visto: 64,
			lacs: []LacunaOut{{From: 31, To: 64, SeqBloco: 64, Causa: "LOSS"}}},
		{nome: "19", s: 30, lote: []bo{{seq: 31, tag: true, marc: protocolo.RegLoss, from: 32, to: 40}}, ack: 30, motivo: MotivoMarcadorInvalido, visto: 31},
		{nome: "20", s: 30, lote: []bo{{seq: 31, boot: 3, tag: true, marc: protocolo.RegBoot, from: 31, to: 30}}, ack: 31, visto: 31},
		{nome: "21", s: 30, lote: []bo{{seq: 31, tag: true, marc: protocolo.RegLoss, from: 31, to: 63}, {seq: 64, tag: true, marc: protocolo.RegLoss, from: 64, to: 64}, {seq: 65, boot: 3, tag: true}},
			ack: 65, visto: 65, lacs: []LacunaOut{{From: 31, To: 63, SeqBloco: 31, Causa: "LOSS"}, {From: 64, To: 64, SeqBloco: 64, Causa: "LOSS"}}},
		{nome: "22", s: 10, lote: []bo{{seq: 11, boot: 1, tag: true}, {seq: 13, boot: 1, tag: true}, {seq: 14, boot: 1}}, ack: 11, motivo: MotivoSalto, visto: 13},
		{nome: "23", s: 10, g: 0, u: 65535, lote: []bo{{seq: 11, boot: 1, tag: true, marc: protocolo.RegBoot, from: 11, to: 10}, {seq: 12, boot: 1, tag: true}},
			ack: 12, visto: 12, gFinal: 1},
		{nome: "24", s: 10, g: 0, u: 7, lote: []bo{{seq: 11, boot: 7, tag: true}, {seq: 12, tag: true, marc: protocolo.RegLoss, from: 12, to: 14}, {seq: 15, boot: 7, tag: true}, {seq: 5, boot: 3, tag: true}},
			ack: 15, dups: 1, visto: 15, lacs: []LacunaOut{{From: 12, To: 14, SeqBloco: 12, Causa: "LOSS"}}},
	}
	for _, c := range casos {
		r := Aplicar(lote(c.lote...), EstadoIn{SeqContiguo: c.s, G: c.g, U: c.u}, cat)
		if r.NovoAck != c.ack || r.Motivo != c.motivo || r.Duplicados != c.dups || r.MaiorVisto != c.visto || r.G != c.gFinal {
			t.Errorf("%s: ack=%d motivo=%q dups=%d visto=%d g=%d", c.nome, r.NovoAck, r.Motivo, r.Duplicados, r.MaiorVisto, r.G)
		}
		if len(r.Lacunas) != len(c.lacs) {
			t.Errorf("%s: lacunas %v", c.nome, r.Lacunas)
			continue
		}
		for i := range c.lacs {
			if r.Lacunas[i] != c.lacs[i] {
				t.Errorf("%s: lacuna %v != %v", c.nome, r.Lacunas[i], c.lacs[i])
			}
		}
	}
}

func TestMalformado(t *testing.T) {
	// marcador + PID no mesmo bloco
	m := protocolo.NovoMontadorBloco(11, 1, 0)
	m.PID(4, 0, 1, 0)
	d := append(append([]byte{}, m.Dados()...), protocolo.MontarMarcador(11, 1, 0, protocolo.RegBoot, 11, 10)[12:]...)
	d[10], d[11] = byte(len(d)), byte(len(d)>>8)
	r := Aplicar(lote(bo{seq: 11, boot: 1, tag: true, dados: d}), EstadoIn{SeqContiguo: 10}, cat)
	if r.Motivo != MotivoRegistroMalformado || r.NovoAck != 10 {
		t.Fatalf("%+v", r)
	}
	if AcaoDe(r.Motivo) != AcaoAguardar || AcaoDe(MotivoSalto) != AcaoReler || AcaoDe("") != AcaoNenhuma {
		t.Fatal("acoes")
	}
}

func TestAtribuirGeracao(t *testing.T) {
	casos := []struct {
		g, u, b, eg, eu uint16
		av              bool
	}{{0, 7, 9, 0, 9, false}, {0, 7, 7, 0, 7, false}, {0, 7, 3, 1, 3, true}, {0, 65535, 1, 1, 1, true}, {2, 5, 0, 2, 5, false}}
	for _, c := range casos {
		g, u, av := atribuirGeracao(c.g, c.u, c.b)
		if g != c.eg || u != c.eu || av != c.av {
			t.Errorf("%+v -> %d %d %v", c, g, u, av)
		}
	}
}
