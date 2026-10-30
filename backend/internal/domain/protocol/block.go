package protocol

import "encoding/binary"

// BlockHeader são os 12 bytes iniciais da área de dados.
type BlockHeader struct {
	Seq    uint32
	BootID uint16
	T0Ms   uint32
	Size   uint16
}

// Block é a mensagem 0x03 decodificada (sem copiar).
type Block struct {
	Hdr  BlockHeader
	Data []byte // V[0:Size]: entra no HMAC e em raw_block.payload
	Tag  []byte // V[Size:]
}

// DecodeBlock valida L == Size + BlockTagSize.
func DecodeBlock(v []byte, p Profile) (Block, error) {
	var b Block
	if len(v) < BlockHeaderSize {
		return b, ErrInvalidSize
	}
	b.Hdr = BlockHeader{
		Seq:    binary.LittleEndian.Uint32(v[0:4]),
		BootID: binary.LittleEndian.Uint16(v[4:6]),
		T0Ms:   binary.LittleEndian.Uint32(v[6:10]),
		Size:   binary.LittleEndian.Uint16(v[10:12]),
	}
	t := int(b.Hdr.Size)
	if t < MinDataSize || t > MaxDataSize || len(v) != t+p.BlockTagSize {
		return b, ErrInvalidSize
	}
	b.Data = v[:t:t]
	b.Tag = v[t:]
	return b, nil
}

// RecordType identifica registros reservados.
type RecordType uint8

const (
	RegLoss RecordType = 0xFD
	RegBoot RecordType = 0xFE
	RegGPS  RecordType = 0xFF
)

type PIDRecord struct {
	PID  uint8
	Dt   uint16
	A, B uint8
}

type GPSRecord struct {
	Dt                  uint16
	LatE7, LonE7, AltMm int32
}

type MarkerRecord struct {
	Type           RecordType
	Dt             uint16
	FromSeq, ToSeq uint32
}

// Content classifica o bloco depois de percorrer os registros.
type Content struct {
	Marker *MarkerRecord
	PIDs   []PIDRecord
	GPS    []GPSRecord
	DtMax  uint16
	DtMin  uint16
}

// ReadRecords percorre dados[12:tamanho] (dados = área de dados inteira).
func ReadRecords(data []byte) (Content, error) {
	var c Content
	if len(data) < MinDataSize {
		return c, ErrMalformedRecord
	}
	r := data[BlockHeaderSize:]
	est := len(r) / 5
	c.PIDs = make([]PIDRecord, 0, est)
	first := true
	touch := func(dt uint16) {
		if first || dt > c.DtMax {
			c.DtMax = dt
		}
		if first || dt < c.DtMin {
			c.DtMin = dt
		}
		first = false
	}
	for len(r) > 0 {
		switch RecordType(r[0]) {
		case RegBoot, RegLoss:
			if len(r) < 11 {
				return c, ErrMalformedRecord
			}
			m := MarkerRecord{
				Type:    RecordType(r[0]),
				Dt:      binary.LittleEndian.Uint16(r[1:3]),
				FromSeq: binary.LittleEndian.Uint32(r[3:7]),
				ToSeq:   binary.LittleEndian.Uint32(r[7:11]),
			}
			if len(data) != MarkerBlockSize || len(r) != 11 || len(c.PIDs) > 0 || len(c.GPS) > 0 {
				return c, ErrMarkerWithOthers
			}
			c.Marker = &m
			touch(m.Dt)
			return c, nil
		case RegGPS:
			if len(r) < 15 {
				return c, ErrMalformedRecord
			}
			g := GPSRecord{
				Dt:    binary.LittleEndian.Uint16(r[1:3]),
				LatE7: int32(binary.LittleEndian.Uint32(r[3:7])),
				LonE7: int32(binary.LittleEndian.Uint32(r[7:11])),
				AltMm: int32(binary.LittleEndian.Uint32(r[11:15])),
			}
			c.GPS = append(c.GPS, g)
			touch(g.Dt)
			r = r[15:]
		default:
			if len(r) < 5 {
				return c, ErrMalformedRecord
			}
			p := PIDRecord{PID: r[0], Dt: binary.LittleEndian.Uint16(r[1:3]), A: r[3], B: r[4]}
			c.PIDs = append(c.PIDs, p)
			touch(p.Dt)
			r = r[5:]
		}
	}
	return c, nil
}

// BlockBuilder monta a área de dados (simulador e testes).
type BlockBuilder struct {
	buf []byte
}

// NewBlockBuilder inicia um bloco com o cabeçalho (tamanho preenchido em Data).
func NewBlockBuilder(seq uint32, bootID uint16, t0Ms uint32) *BlockBuilder {
	b := make([]byte, BlockHeaderSize, MaxDataSize)
	binary.LittleEndian.PutUint32(b[0:], seq)
	binary.LittleEndian.PutUint16(b[4:], bootID)
	binary.LittleEndian.PutUint32(b[6:], t0Ms)
	return &BlockBuilder{buf: b}
}

// PID acrescenta um registro de PID; false se não couber.
func (m *BlockBuilder) PID(pid uint8, dt uint16, a, b uint8) bool {
	if len(m.buf)+5 > MaxDataSize {
		return false
	}
	m.buf = append(m.buf, pid, byte(dt), byte(dt>>8), a, b)
	return true
}

// GPS acrescenta um registro de GPS; false se não couber.
func (m *BlockBuilder) GPS(dt uint16, lat, lon, alt int32) bool {
	if len(m.buf)+15 > MaxDataSize {
		return false
	}
	m.buf = append(m.buf, byte(RegGPS), byte(dt), byte(dt>>8))
	m.buf = binary.LittleEndian.AppendUint32(m.buf, uint32(lat))
	m.buf = binary.LittleEndian.AppendUint32(m.buf, uint32(lon))
	m.buf = binary.LittleEndian.AppendUint32(m.buf, uint32(alt))
	return true
}

// Data fecha o bloco, escrevendo o tamanho no cabeçalho.
func (m *BlockBuilder) Data() []byte {
	binary.LittleEndian.PutUint16(m.buf[10:], uint16(len(m.buf)))
	return m.buf
}

// BuildMarker monta a área de dados de um bloco BOOT/LOSS (23 B).
func BuildMarker(seq uint32, bootID uint16, t0 uint32, typ RecordType, from, to uint32) []byte {
	m := NewBlockBuilder(seq, bootID, t0)
	m.buf = append(m.buf, byte(typ), 0, 0)
	m.buf = binary.LittleEndian.AppendUint32(m.buf, from)
	m.buf = binary.LittleEndian.AppendUint32(m.buf, to)
	return m.Data()
}

// BuildBlockTLV devolve o TLV 0x03 com dados + tag.
func BuildBlockTLV(data, tag []byte) []byte {
	v := make([]byte, 0, len(data)+len(tag))
	v = append(v, data...)
	v = append(v, tag...)
	return AppendTLV(nil, BlockType, v)
}
