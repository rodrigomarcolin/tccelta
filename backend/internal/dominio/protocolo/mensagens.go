package protocolo

import "encoding/binary"

// Desafio é a mensagem 0x01 (servidor -> dongle).
type Desafio struct {
	CS, AckSeq, CConf uint32
	NS                [16]byte
	Flags             uint8
	DiagMin           uint16 // só se Flags&1
}

// Payload devolve V sem a tag.
func (d Desafio) Payload() []byte {
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

// Montar devolve o TLV completo (T|L|V) com a tag de 16 B.
func (d Desafio) Montar(tag []byte) ([]byte, error) {
	if d.Flags&^1 != 0 {
		return nil, ErrFlagsReservadas
	}
	if len(tag) != TamTagAuth {
		return nil, ErrTamanhoInvalido
	}
	v := append(d.Payload(), tag...)
	return AnexarTLV(nil, TipoDesafio, v), nil
}

// DecodificarDesafio decodifica V (45 ou 47 B) e devolve a tag.
func DecodificarDesafio(v []byte) (Desafio, []byte, error) {
	var d Desafio
	if len(v) != LDesafio && len(v) != LDesafioDiag {
		return d, nil, ErrTamanhoInvalido
	}
	d.CS = binary.LittleEndian.Uint32(v[0:4])
	copy(d.NS[:], v[4:20])
	d.AckSeq = binary.LittleEndian.Uint32(v[20:24])
	d.CConf = binary.LittleEndian.Uint32(v[24:28])
	d.Flags = v[28]
	if d.Flags&^1 != 0 {
		return d, nil, ErrFlagsReservadas
	}
	n := 29
	if d.Flags&1 != 0 {
		if len(v) != LDesafioDiag {
			return d, nil, ErrTamanhoInvalido
		}
		d.DiagMin = binary.LittleEndian.Uint16(v[29:31])
		n = 31
	} else if len(v) != LDesafio {
		return d, nil, ErrTamanhoInvalido
	}
	return d, v[n:], nil
}

// Resposta é a mensagem 0x02 (dongle -> servidor).
type Resposta struct {
	CS, TailSeq, HeadSeq, TMs uint32
	NS                        [16]byte
	BootID                    uint16
}

// DecodificarResposta decodifica V (50 B).
func DecodificarResposta(v []byte) (r Resposta, payload, tag []byte, err error) {
	if len(v) != LResposta {
		return r, nil, nil, ErrTamanhoInvalido
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
func (r Resposta) Payload() []byte {
	p := make([]byte, 0, 34)
	p = binary.LittleEndian.AppendUint32(p, r.CS)
	p = append(p, r.NS[:]...)
	p = binary.LittleEndian.AppendUint32(p, r.TailSeq)
	p = binary.LittleEndian.AppendUint32(p, r.HeadSeq)
	p = binary.LittleEndian.AppendUint16(p, r.BootID)
	p = binary.LittleEndian.AppendUint32(p, r.TMs)
	return p
}

// Montar devolve o TLV completo (simulador).
func (r Resposta) Montar(tag []byte) []byte {
	return AnexarTLV(nil, TipoResposta, append(r.Payload(), tag...))
}

// Ack é a mensagem 0x04 (servidor -> dongle).
type Ack struct {
	AckSeq, CConf uint32
	Flags         uint8
}

// FlagAckAncoraFalhou: a âncora de tempo falhou, repetir o handshake.
const FlagAckAncoraFalhou uint8 = 1 << 0

// Payload devolve V sem a tag (9 B).
func (a Ack) Payload() []byte {
	p := make([]byte, 0, 9)
	p = binary.LittleEndian.AppendUint32(p, a.AckSeq)
	p = binary.LittleEndian.AppendUint32(p, a.CConf)
	return append(p, a.Flags)
}

// Montar devolve o TLV completo com a tag de 16 B.
func (a Ack) Montar(tag []byte) []byte {
	return AnexarTLV(nil, TipoAck, append(a.Payload(), tag...))
}

// DecodificarAck decodifica V (25 B) e devolve a tag.
func DecodificarAck(v []byte) (Ack, []byte, error) {
	var a Ack
	if len(v) != LAck {
		return a, nil, ErrTamanhoInvalido
	}
	a.AckSeq = binary.LittleEndian.Uint32(v[0:4])
	a.CConf = binary.LittleEndian.Uint32(v[4:8])
	a.Flags = v[8]
	if a.Flags&^1 != 0 {
		return a, nil, ErrFlagsReservadas
	}
	return a, v[9:], nil
}
