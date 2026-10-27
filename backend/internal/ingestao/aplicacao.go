// Package ingestao implementa o lote de blocos (spec 05).
package ingestao

import (
	"tccelta/backend/internal/pids"
	"tccelta/backend/internal/protocolo"
)

// Motivo da interrupção de um lote.
type Motivo string

const (
	MotivoNenhum             Motivo = ""
	MotivoTagInvalida        Motivo = "tag_invalida"
	MotivoSalto              Motivo = "salto"
	MotivoMarcadorInvalido   Motivo = "marcador_invalido"
	MotivoPIDDesconhecido    Motivo = "pid_desconhecido"
	MotivoRegistroMalformado Motivo = "registro_malformado"
)

// Acao orienta o buffer do app.
type Acao string

const (
	AcaoNenhuma  Acao = "nenhuma"
	AcaoReler    Acao = "reler"
	AcaoAguardar Acao = "aguardar"
)

// AcaoDe mapeia o motivo para a ação.
func AcaoDe(m Motivo) Acao {
	switch m {
	case MotivoNenhum:
		return AcaoNenhuma
	case MotivoPIDDesconhecido, MotivoRegistroMalformado:
		return AcaoAguardar
	}
	return AcaoReler
}

// BlocoIn é um bloco do lote com o resultado da verificação.
type BlocoIn struct {
	B      protocolo.Bloco
	Valido bool // tag conferida (só significativo se Verificado)
	// Verificado: seq acima do snapshot (passou pelo KMS).
	Verificado bool
}

// Seq do bloco.
func (b BlocoIn) Seq() uint32 { return b.B.Cab.Seq }

// EstadoIn é o estado de partida da aplicação.
type EstadoIn struct {
	SeqContiguo uint32
	G, U        uint16
}

// Aceito é um bloco que entra em bloco_bruto.
type Aceito struct {
	B        protocolo.Bloco
	Cont     protocolo.Conteudo
	Geracao  uint16
	Marcador string // "", "BOOT", "LOSS"
}

// LacunaOut é uma lacuna BOOT/LOSS a registrar.
type LacunaOut struct {
	From, To, SeqBloco uint32
	Causa              string
}

// Rejeicao é um bloco recusado.
type Rejeicao struct {
	B       protocolo.Bloco
	Motivo  Motivo
	Detalhe string
	// PIDDesconhecido carrega o PID para o alerta.
	PID int
}

// ResultadoAplicacao é a saída pura de aplicar.
type ResultadoAplicacao struct {
	Aceitos        []Aceito
	Lacunas        []LacunaOut
	Rejeitados     []Rejeicao
	Motivo         Motivo
	NovoAck        uint32
	MaiorVisto     uint32
	G, U           uint16
	Duplicados     int
	GeracaoAvancou bool
}

// atribuirGeracao: boot_id 0 não participa; um boot_id menor que U é a volta do u16.
func atribuirGeracao(g, u, bootID uint16) (ger, novoU uint16, avancou bool) {
	if bootID == 0 {
		return g, u, false
	}
	if bootID < u {
		return g + 1, bootID, true
	}
	return g, bootID, false
}

// aplicar executa as regras do lote (Go-Back-N estrito, BOOT/LOSS). Pura.
func aplicar(blocos []BlocoIn, e EstadoIn, cat *pids.Catalogo) ResultadoAplicacao {
	r := ResultadoAplicacao{G: e.G, U: e.U}
	exp := e.SeqContiguo + 1
	for _, b := range blocos {
		if b.Verificado && b.Valido && b.Seq() > r.MaiorVisto {
			r.MaiorVisto = b.Seq()
		}
	}
	// todo bloco verificado com tag inválida vai para rejeitados
	for _, b := range blocos {
		if b.Verificado && !b.Valido && b.Seq() >= exp {
			r.Rejeitados = append(r.Rejeitados, Rejeicao{B: b.B, Motivo: MotivoTagInvalida})
		}
	}
laco:
	for _, b := range blocos {
		seq := b.Seq()
		if seq < exp {
			r.Duplicados++
			continue
		}
		if !b.Valido {
			r.Motivo = MotivoTagInvalida
			break
		}
		c, err := protocolo.LerRegistros(b.B.Dados)
		if err != nil {
			r.Rejeitados = append(r.Rejeitados, Rejeicao{B: b.B, Motivo: MotivoRegistroMalformado, Detalhe: err.Error()})
			r.Motivo = MotivoRegistroMalformado
			break
		}
		a := Aceito{B: b.B, Cont: c}
		switch {
		case c.Marcador != nil && c.Marcador.Tipo == protocolo.RegBoot:
			m := c.Marcador
			if !(m.ToSeq == seq-1 && m.FromSeq <= exp && seq >= exp) {
				r.Rejeitados = append(r.Rejeitados, Rejeicao{B: b.B, Motivo: MotivoMarcadorInvalido, Detalhe: "BOOT fora da regra"})
				r.Motivo = MotivoMarcadorInvalido
				break laco
			}
			if exp <= m.ToSeq {
				r.Lacunas = append(r.Lacunas, LacunaOut{From: exp, To: m.ToSeq, SeqBloco: seq, Causa: "BOOT"})
			}
			exp = seq + 1
			a.Marcador = "BOOT"
			a.Geracao, r.U, _ = r.atribuir(b.B.Cab.BootID)
		case c.Marcador != nil:
			m := c.Marcador
			if !(m.FromSeq == exp && m.FromSeq <= seq && seq <= m.ToSeq) {
				r.Rejeitados = append(r.Rejeitados, Rejeicao{B: b.B, Motivo: MotivoMarcadorInvalido, Detalhe: "LOSS fora da regra"})
				r.Motivo = MotivoMarcadorInvalido
				break laco
			}
			r.Lacunas = append(r.Lacunas, LacunaOut{From: m.FromSeq, To: m.ToSeq, SeqBloco: seq, Causa: "LOSS"})
			exp = m.ToSeq + 1
			a.Marcador = "LOSS"
			a.Geracao = r.G
		default:
			if seq != exp {
				r.Motivo = MotivoSalto
				break laco
			}
			for _, p := range c.PIDs {
				if _, ok := cat.Obter(p.PID); !ok {
					r.Rejeitados = append(r.Rejeitados, Rejeicao{B: b.B, Motivo: MotivoPIDDesconhecido, PID: int(p.PID)})
					r.Motivo = MotivoPIDDesconhecido
					break laco
				}
			}
			a.Geracao, r.U, _ = r.atribuir(b.B.Cab.BootID)
			exp = seq + 1
		}
		r.Aceitos = append(r.Aceitos, a)
	}
	r.NovoAck = exp - 1
	return r
}

// atribuir aplica atribuirGeracao ao estado (G, U) do resultado.
func (r *ResultadoAplicacao) atribuir(bootID uint16) (uint16, uint16, bool) {
	ger, u, av := atribuirGeracao(r.G, r.U, bootID)
	if av {
		r.GeracaoAvancou = true
	}
	r.G = ger
	return ger, u, av
}
