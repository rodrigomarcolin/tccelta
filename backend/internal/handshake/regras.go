// Package handshake implementa o desafio e a resposta (spec 04).
package handshake

import (
	"errors"
	"time"
)

// Faixa é um intervalo fechado de seq.
type Faixa struct{ Ini, Fim uint32 }

// Estado é o recorte de dongle_estado usado pelas regras.
type Estado struct {
	SeqContiguo, HeadSeq, CConf uint32
	GeracaoBoot, UltimoBootID   uint16
}

// Erros de regra.
var (
	ErrDesafiosPendentes = errors.New("desafios_pendentes")
	ErrDesafioInvalido   = errors.New("desafio_invalido")
	ErrCSAntigo          = errors.New("c_s_antigo")
	ErrTagInvalida       = errors.New("tag_invalida")
	ErrRespostaInconsist = errors.New("resposta_inconsistente")
)

// aplicarTail aplica a consistência e a regra do TAIL_seq (OVERFLOW).
func aplicarTail(e Estado, tail, head uint32) (ack uint32, perda *Faixa, err error) {
	if uint64(tail) > uint64(head)+1 || head < e.HeadSeq {
		return 0, nil, ErrRespostaInconsist
	}
	ack = e.SeqContiguo
	if uint64(tail) > uint64(ack)+1 {
		perda = &Faixa{Ini: ack + 1, Fim: tail - 1}
		ack = tail - 1
	}
	return ack, perda, nil
}

// avaliarAncora: âncora só se 0 <= rtt <= max; rtt negativo gera alerta.
func avaliarAncora(s1, s2 time.Time, max time.Duration) (ok bool, alertaRelogio bool) {
	rtt := s2.Sub(s1)
	if rtt < 0 {
		return false, true
	}
	return rtt <= max, false
}

// geracaoDoHandshake infere a geração do boot_id sem gravar o estado.
func geracaoDoHandshake(e Estado, bootID uint16) uint16 {
	if bootID == 0 || bootID >= e.UltimoBootID {
		return e.GeracaoBoot
	}
	return e.GeracaoBoot + 1
}

// truncarMicro arredonda S1 para baixo no microssegundo; tetoMicro arredonda S2 para cima.
func truncarMicro(t time.Time) time.Time { return t.Truncate(time.Microsecond) }

func tetoMicro(t time.Time) time.Time {
	r := t.Truncate(time.Microsecond)
	if r.Before(t) {
		r = r.Add(time.Microsecond)
	}
	return r
}
