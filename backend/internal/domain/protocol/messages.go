package protocol

import "encoding/binary"

// Challenge é a mensagem 0x01 (servidor -> dongle).
type Challenge struct {
	CS, AckSeq, CConf uint32
	NS                [16]byte
	Flags             uint8
	DiagMin           uint16 // só se Flags&1
}

// Payload devolve V sem a tag.
func (d Challenge) Payload() []byte {
	p := make([]byte, 0, 31)
	p = binary.LittleEndian.AppendUint32(p, d.CS)
	p = append(p, d.NS[:]...)
	p = binary.LittleEndian.AppendUint32(p, d.AckSeq)
	p = binary.LittleEndian.AppendUint32(p, d.CConf)
	p = append(p, d.Flags)
	if d.Flags&1 != 0 {
		p = binary.LittleEndian.AppendUint16(p, d.DiagMin)
	}
	return p
}

// Build devolve o TLV completo (T|L|V) com a tag de 16 B.
func (d Challenge) Build(tag []byte) ([]byte, error) {
	if d.Flags&^1 != 0 {
		return nil, ErrReservedFlags
	}
	if len(tag) != AuthTagSize {
		return nil, ErrInvalidSize
	}
	v := append(d.Payload(), tag...)
	return AppendTLV(nil, ChallengeType, v), nil
}

// DecodeChallenge decodifica V (45 ou 47 B) e devolve a tag.
func DecodeChallenge(v []byte) (Challenge, []byte, error) {
	var d Challenge
	if len(v) != LChallenge && len(v) != LChallengeDiag {
		return d, nil, ErrInvalidSize
	}
	d.CS = binary.LittleEndian.Uint32(v[0:4])
	copy(d.NS[:], v[4:20])
	d.AckSeq = binary.LittleEndian.Uint32(v[20:24])
	d.CConf = binary.LittleEndian.Uint32(v[24:28])
	d.Flags = v[28]
	if d.Flags&^1 != 0 {
		return d, nil, ErrReservedFlags
	}
	n := 29
	if d.Flags&1 != 0 {
		if len(v) != LChallengeDiag {
			return d, nil, ErrInvalidSize
		}
		d.DiagMin = binary.LittleEndian.Uint16(v[29:31])
		n = 31
	} else if len(v) != LChallenge {
		return d, nil, ErrInvalidSize
	}
	return d, v[n:], nil
}

// Response é a mensagem 0x02 (dongle -> servidor).
type Response struct {
	CS, TailSeq, HeadSeq, TMs uint32
	NS                        [16]byte
	BootID                    uint16
}

// DecodeResponse decodifica V (50 B).
func DecodeResponse(v []byte) (r Response, payload, tag []byte, err error) {
	if len(v) != LResponse {
		return r, nil, nil, ErrInvalidSize
	}
	r.CS = binary.LittleEndian.Uint32(v[0:4])
	copy(r.NS[:], v[4:20])
	r.TailSeq = binary.LittleEndian.Uint32(v[20:24])
	r.HeadSeq = binary.LittleEndian.Uint32(v[24:28])
	r.BootID = binary.LittleEndian.Uint16(v[28:30])
	r.TMs = binary.LittleEndian.Uint32(v[30:34])
	return r, v[:34], v[34:], nil
}

// Payload devolve V sem a tag.
func (r Response) Payload() []byte {
	p := make([]byte, 0, 34)
	p = binary.LittleEndian.AppendUint32(p, r.CS)
	p = append(p, r.NS[:]...)
	p = binary.LittleEndian.AppendUint32(p, r.TailSeq)
	p = binary.LittleEndian.AppendUint32(p, r.HeadSeq)
	p = binary.LittleEndian.AppendUint16(p, r.BootID)
	p = binary.LittleEndian.AppendUint32(p, r.TMs)
	return p
}

// Build devolve o TLV completo (simulador).
func (r Response) Build(tag []byte) []byte {
	return AppendTLV(nil, ResponseType, append(r.Payload(), tag...))
}

// Ack é a mensagem 0x04 (servidor -> dongle).
type Ack struct {
	AckSeq, CConf uint32
	Flags         uint8
}

// FlagAckAnchorFailed: a âncora de tempo falhou, repetir o handshake.
const FlagAckAnchorFailed uint8 = 1 << 0

// Payload devolve V sem a tag (9 B).
func (a Ack) Payload() []byte {
	p := make([]byte, 0, 9)
	p = binary.LittleEndian.AppendUint32(p, a.AckSeq)
	p = binary.LittleEndian.AppendUint32(p, a.CConf)
	return append(p, a.Flags)
}

// Build devolve o TLV completo com a tag de 16 B.
func (a Ack) Build(tag []byte) []byte {
	return AppendTLV(nil, AckType, append(a.Payload(), tag...))
}

// DecodeAck decodifica V (25 B) e devolve a tag.
func DecodeAck(v []byte) (Ack, []byte, error) {
	var a Ack
	if len(v) != LAck {
		return a, nil, ErrInvalidSize
	}
	a.AckSeq = binary.LittleEndian.Uint32(v[0:4])
	a.CConf = binary.LittleEndian.Uint32(v[4:8])
	a.Flags = v[8]
	if a.Flags&^1 != 0 {
		return a, nil, ErrReservedFlags
	}
	return a, v[9:], nil
}
