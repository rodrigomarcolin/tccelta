package protocolo

import "encoding/binary"

// CabecalhoBloco são os 12 bytes iniciais da área de dados.
type CabecalhoBloco struct {
	Seq     uint32
	BootID  uint16
	T0Ms    uint32
	Tamanho uint16
}

// Bloco é a mensagem 0x03 decodificada (sem copiar).
type Bloco struct {
	Cab   CabecalhoBloco
	Dados []byte // V[0:Tamanho]: entra no HMAC e em bloco_bruto.payload
	Tag   []byte // V[Tamanho:]
}

// DecodificarBloco valida L == Tamanho + TamTagBloco.
func DecodificarBloco(v []byte, p Perfil) (Bloco, error) {
	var b Bloco
	if len(v) < TamCabBloco {
		return b, ErrTamanhoInvalido
	}
	b.Cab = CabecalhoBloco{
		Seq:     binary.LittleEndian.Uint32(v[0:4]),
		BootID:  binary.LittleEndian.Uint16(v[4:6]),
		T0Ms:    binary.LittleEndian.Uint32(v[6:10]),
		Tamanho: binary.LittleEndian.Uint16(v[10:12]),
	}
	t := int(b.Cab.Tamanho)
	if t < TamMinDados || t > TamMaxDados || len(v) != t+p.TamTagBloco {
		return b, ErrTamanhoInvalido
	}
	b.Dados = v[:t:t]
	b.Tag = v[t:]
	return b, nil
}

// TipoRegistro identifica registros reservados.
type TipoRegistro uint8

const (
	RegLoss TipoRegistro = 0xFD
	RegBoot TipoRegistro = 0xFE
	RegGPS  TipoRegistro = 0xFF
)

type RegistroPID struct {
	PID  uint8
	Dt   uint16
	A, B uint8
}

type RegistroGPS struct {
	Dt                  uint16
	LatE7, LonE7, AltMm int32
}

type RegistroMarcador struct {
	Tipo           TipoRegistro
	Dt             uint16
	FromSeq, ToSeq uint32
}

// Conteudo classifica o bloco depois de percorrer os registros.
type Conteudo struct {
	Marcador *RegistroMarcador
	PIDs     []RegistroPID
	GPS      []RegistroGPS
	DtMax    uint16
	DtMin    uint16
}

// LerRegistros percorre dados[12:tamanho] (dados = área de dados inteira).
func LerRegistros(dados []byte) (Conteudo, error) {
	var c Conteudo
	if len(dados) < TamMinDados {
		return c, ErrRegistroMalformado
	}
	r := dados[TamCabBloco:]
	est := len(r) / 5
	c.PIDs = make([]RegistroPID, 0, est)
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
		switch TipoRegistro(r[0]) {
		case RegBoot, RegLoss:
			if len(r) < 11 {
				return c, ErrRegistroMalformado
			}
			m := RegistroMarcador{
				Tipo:    TipoRegistro(r[0]),
				Dt:      binary.LittleEndian.Uint16(r[1:3]),
				FromSeq: binary.LittleEndian.Uint32(r[3:7]),
				ToSeq:   binary.LittleEndian.Uint32(r[7:11]),
			}
			if len(dados) != TamBlocoMarcador || len(r) != 11 || len(c.PIDs) > 0 || len(c.GPS) > 0 {
				return c, ErrMarcadorComOutros
			}
			c.Marcador = &m
			touch(m.Dt)
			return c, nil
		case RegGPS:
			if len(r) < 15 {
				return c, ErrRegistroMalformado
			}
			g := RegistroGPS{
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
				return c, ErrRegistroMalformado
			}
			p := RegistroPID{PID: r[0], Dt: binary.LittleEndian.Uint16(r[1:3]), A: r[3], B: r[4]}
			c.PIDs = append(c.PIDs, p)
			touch(p.Dt)
			r = r[5:]
		}
	}
	return c, nil
}

// MontadorBloco monta a área de dados (simulador e testes).
type MontadorBloco struct {
	buf []byte
}

// NovoMontadorBloco inicia um bloco com o cabeçalho (tamanho preenchido em Dados).
func NovoMontadorBloco(seq uint32, bootID uint16, t0Ms uint32) *MontadorBloco {
	b := make([]byte, TamCabBloco, TamMaxDados)
	binary.LittleEndian.PutUint32(b[0:], seq)
	binary.LittleEndian.PutUint16(b[4:], bootID)
	binary.LittleEndian.PutUint32(b[6:], t0Ms)
	return &MontadorBloco{buf: b}
}

// PID acrescenta um registro de PID; false se não couber.
func (m *MontadorBloco) PID(pid uint8, dt uint16, a, b uint8) bool {
	if len(m.buf)+5 > TamMaxDados {
		return false
	}
	m.buf = append(m.buf, pid, byte(dt), byte(dt>>8), a, b)
	return true
}

// GPS acrescenta um registro de GPS; false se não couber.
func (m *MontadorBloco) GPS(dt uint16, lat, lon, alt int32) bool {
	if len(m.buf)+15 > TamMaxDados {
		return false
	}
	m.buf = append(m.buf, byte(RegGPS), byte(dt), byte(dt>>8))
	m.buf = binary.LittleEndian.AppendUint32(m.buf, uint32(lat))
	m.buf = binary.LittleEndian.AppendUint32(m.buf, uint32(lon))
	m.buf = binary.LittleEndian.AppendUint32(m.buf, uint32(alt))
	return true
}

// Dados fecha o bloco, escrevendo o tamanho no cabeçalho.
func (m *MontadorBloco) Dados() []byte {
	binary.LittleEndian.PutUint16(m.buf[10:], uint16(len(m.buf)))
	return m.buf
}

// MontarMarcador monta a área de dados de um bloco BOOT/LOSS (23 B).
func MontarMarcador(seq uint32, bootID uint16, t0 uint32, tipo TipoRegistro, from, to uint32) []byte {
	m := NovoMontadorBloco(seq, bootID, t0)
	m.buf = append(m.buf, byte(tipo), 0, 0)
	m.buf = binary.LittleEndian.AppendUint32(m.buf, from)
	m.buf = binary.LittleEndian.AppendUint32(m.buf, to)
	return m.Dados()
}

// MontarBlocoTLV devolve o TLV 0x03 com dados + tag.
func MontarBlocoTLV(dados, tag []byte) []byte {
	v := make([]byte, 0, len(dados)+len(tag))
	v = append(v, dados...)
	v = append(v, tag...)
	return AnexarTLV(nil, TipoBloco, v)
}
