// Package ingestion reúne as regras puras do lote de blocos (spec 05).
package ingestion

import (
	"tccelta/backend/internal/domain/pid"
	"tccelta/backend/internal/domain/protocol"
)

// Reason da interrupção de um lote.
type Reason string

const (
	ReasonNone            Reason = ""
	ReasonInvalidTag      Reason = "invalid_tag"
	ReasonJump            Reason = "jump"
	ReasonInvalidMarker   Reason = "invalid_marker"
	ReasonUnknownPID      Reason = "unknown_pid"
	ReasonMalformedRecord Reason = "malformed_record"
)

// Action orienta o buffer do app. Os valores são contrato de fio (cabeçalho X-Lote-Acao), por isso em português.
type Action string

const (
	ActionNone   Action = "nenhuma"
	ActionReread Action = "reler"
	ActionWait   Action = "aguardar"
)

// ActionOf mapeia o motivo para a ação.
func ActionOf(m Reason) Action {
	switch m {
	case ReasonNone:
		return ActionNone
	case ReasonUnknownPID, ReasonMalformedRecord:
		return ActionWait
	}
	return ActionReread
}

// BlockIn é um bloco do lote com o resultado da verificação.
type BlockIn struct {
	B     protocol.Block
	Valid bool // tag conferida (só significativo se Verified)
	// Verified: seq acima do snapshot (passou pelo KMS).
	Verified bool
}

// Seq do bloco.
func (b BlockIn) Seq() uint32 { return b.B.Hdr.Seq }

// StateIn é o estado de partida da aplicação.
type StateIn struct {
	ContiguousSeq uint32
	G, U          uint16
}

// Accepted é um bloco que entra em raw_block.
type Accepted struct {
	B          protocol.Block
	Content    protocol.Content
	Generation uint16
	Marker     string // "", "BOOT", "LOSS"
}

// GapOut é uma lacuna BOOT/LOSS a registrar.
type GapOut struct {
	From, To, BlockSeq uint32
	Cause              string
}

// Rejection é um bloco recusado.
type Rejection struct {
	B      protocol.Block
	Reason Reason
	Detail string
	// PIDDesconhecido carrega o PID para o alerta.
	PID int
}

// ApplyResult é a saída pura de aplicar.
type ApplyResult struct {
	Accepted           []Accepted
	Gaps               []GapOut
	Rejected           []Rejection
	Reason             Reason
	NewAck             uint32
	MaxSeen            uint32
	G, U               uint16
	Duplicates         int
	GenerationAdvanced bool
}

// assignGeneration: boot_id 0 não participa; um boot_id menor que U é a volta do u16.
func assignGeneration(g, u, bootID uint16) (gen, newU uint16, advanced bool) {
	if bootID == 0 {
		return g, u, false
	}
	if bootID < u {
		return g + 1, bootID, true
	}
	return g, bootID, false
}

// Apply executa as regras do lote (Go-Back-N estrito, BOOT/LOSS). Pura.
func Apply(blocks []BlockIn, e StateIn, cat pid.Query) ApplyResult {
	r := ApplyResult{G: e.G, U: e.U}
	exp := e.ContiguousSeq + 1
	for _, b := range blocks {
		if b.Verified && b.Valid && b.Seq() > r.MaxSeen {
			r.MaxSeen = b.Seq()
		}
	}
	// todo bloco verificado com tag inválida vai para rejeitados
	for _, b := range blocks {
		if b.Verified && !b.Valid && b.Seq() >= exp {
			r.Rejected = append(r.Rejected, Rejection{B: b.B, Reason: ReasonInvalidTag})
		}
	}
loop:
	for _, b := range blocks {
		seq := b.Seq()
		if seq < exp {
			r.Duplicates++
			continue
		}
		if !b.Valid {
			r.Reason = ReasonInvalidTag
			break
		}
		c, err := protocol.ReadRecords(b.B.Data)
		if err != nil {
			r.Rejected = append(r.Rejected, Rejection{B: b.B, Reason: ReasonMalformedRecord, Detail: err.Error()})
			r.Reason = ReasonMalformedRecord
			break
		}
		a := Accepted{B: b.B, Content: c}
		switch {
		case c.Marker != nil && c.Marker.Type == protocol.RegBoot:
			m := c.Marker
			if m.ToSeq != seq-1 || m.FromSeq > exp || seq < exp {
				r.Rejected = append(r.Rejected, Rejection{B: b.B, Reason: ReasonInvalidMarker, Detail: "BOOT fora da regra"})
				r.Reason = ReasonInvalidMarker
				break loop
			}
			if exp <= m.ToSeq {
				r.Gaps = append(r.Gaps, GapOut{From: exp, To: m.ToSeq, BlockSeq: seq, Cause: "BOOT"})
			}
			exp = seq + 1
			a.Marker = "BOOT"
			a.Generation, r.U, _ = r.assign(b.B.Hdr.BootID)
		case c.Marker != nil:
			m := c.Marker
			if m.FromSeq != exp || m.FromSeq > seq || seq > m.ToSeq {
				r.Rejected = append(r.Rejected, Rejection{B: b.B, Reason: ReasonInvalidMarker, Detail: "LOSS fora da regra"})
				r.Reason = ReasonInvalidMarker
				break loop
			}
			r.Gaps = append(r.Gaps, GapOut{From: m.FromSeq, To: m.ToSeq, BlockSeq: seq, Cause: "LOSS"})
			exp = m.ToSeq + 1
			a.Marker = "LOSS"
			a.Generation = r.G
		default:
			if seq != exp {
				r.Reason = ReasonJump
				break loop
			}
			for _, p := range c.PIDs {
				if _, ok := cat.Get(p.PID); !ok {
					r.Rejected = append(r.Rejected, Rejection{B: b.B, Reason: ReasonUnknownPID, PID: int(p.PID)})
					r.Reason = ReasonUnknownPID
					break loop
				}
			}
			a.Generation, r.U, _ = r.assign(b.B.Hdr.BootID)
			exp = seq + 1
		}
		r.Accepted = append(r.Accepted, a)
	}
	r.NewAck = exp - 1
	return r
}

// assign aplica assignGeneration ao estado (G, U) do resultado.
func (r *ApplyResult) assign(bootID uint16) (uint16, uint16, bool) {
	gen, u, av := assignGeneration(r.G, r.U, bootID)
	if av {
		r.GenerationAdvanced = true
	}
	r.G = gen
	return gen, u, av
}
