// Package handshake reúne as regras puras do desafio e da resposta (spec 04).
package handshake

import (
	"errors"
	"time"
)

// Range é um intervalo fechado de seq.
type Range struct{ Start, End uint32 }

// State é o recorte de dongle_state usado pelas regras.
type State struct {
	ContiguousSeq, HeadSeq, CConf uint32
	BootGeneration, LastBootID    uint16
}

// Erros de regra.
var (
	ErrPendingChallenges    = errors.New("desafios_pendentes")
	ErrInvalidChallenge     = errors.New("desafio_invalido")
	ErrStaleCS              = errors.New("c_s_antigo")
	ErrInvalidTag           = errors.New("tag_invalida")
	ErrInconsistentResponse = errors.New("resposta_inconsistente")
)

// ApplyTail aplica a consistência e a regra do TAIL_seq (OVERFLOW).
func ApplyTail(e State, tail, head uint32) (ack uint32, loss *Range, err error) {
	if uint64(tail) > uint64(head)+1 || head < e.HeadSeq {
		return 0, nil, ErrInconsistentResponse
	}
	ack = e.ContiguousSeq
	if uint64(tail) > uint64(ack)+1 {
		loss = &Range{Start: ack + 1, End: tail - 1}
		ack = tail - 1
	}
	return ack, loss, nil
}

// EvaluateAnchor: âncora só se 0 <= rtt <= max; rtt negativo gera alerta.
func EvaluateAnchor(s1, s2 time.Time, max time.Duration) (ok bool, clockAlert bool) {
	rtt := s2.Sub(s1)
	if rtt < 0 {
		return false, true
	}
	return rtt <= max, false
}

// HandshakeGeneration infere a geração do boot_id sem gravar o estado.
func HandshakeGeneration(e State, bootID uint16) uint16 {
	if bootID == 0 || bootID >= e.LastBootID {
		return e.BootGeneration
	}
	return e.BootGeneration + 1
}

// TruncateMicro arredonda S1 para baixo no microssegundo; MicroCeiling arredonda S2 para cima.
func TruncateMicro(t time.Time) time.Time { return t.Truncate(time.Microsecond) }

func MicroCeiling(t time.Time) time.Time {
	r := t.Truncate(time.Microsecond)
	if r.Before(t) {
		r = r.Add(time.Microsecond)
	}
	return r
}
