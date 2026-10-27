// Package simulador contém dongles e apps virtuais que usam o mesmo pacote protocolo da API.
package simulador

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"sync"
	"time"

	"tccelta/backend/internal/dominio/protocolo"
)

// Chaves de um dongle virtual (as mesmas importadas no Vault de teste).
type Chaves struct {
	Dev         protocolo.DevID
	KAuth, KRec []byte
}

// ConfigDongle parametriza o comportamento do firmware simulado.
type ConfigDongle struct {
	CapacidadeFlash int     // blocos
	DerivaPPM       float64 // deriva do cristal
	BootInicial     uint16  // boot_id do primeiro boot
}

type blocoFlash struct {
	seq        uint32
	boot       uint16
	tlv        []byte
	registro   int // nº de registros de PID+GPS (0 em marcador)
	boot_      bool
	from       uint32
	corrompido bool
}

// DongleVirtual reproduz o lado dongle do protocolo (spec 10).
type DongleVirtual struct {
	mu       sync.Mutex
	ch       Chaves
	cfg      ConfigDongle
	bootID   uint16
	seq      uint32 // último seq atribuído
	ackLocal uint32
	ultimoCS uint32
	inicio   time.Time
	flash    []blocoFlash
	// Registros por seq (para as verificações).
	Registros map[uint32]int
	Gerados   int
	n         int
}

// NovoDongle cria um dongle e executa o primeiro boot (gera o BOOT de faixa vazia).
func NovoDongle(ch Chaves, cfg ConfigDongle) *DongleVirtual {
	if cfg.CapacidadeFlash <= 0 {
		cfg.CapacidadeFlash = 1000000
	}
	if cfg.BootInicial == 0 {
		cfg.BootInicial = 1
	}
	d := &DongleVirtual{ch: ch, cfg: cfg, bootID: cfg.BootInicial - 1, Registros: map[uint32]int{}}
	d.boot(0)
	return d
}

func (d *DongleVirtual) tMs(t time.Time) uint32 {
	ms := float64(t.Sub(d.inicio).Milliseconds()) * (1 + d.cfg.DerivaPPM*1e-6)
	return uint32(int64(ms))
}

func (d *DongleVirtual) assinar(dados []byte) []byte {
	m := hmac.New(sha256.New, d.ch.KRec)
	m.Write(protocolo.EntradaTag(protocolo.TipoBloco, d.ch.Dev, dados))
	return m.Sum(nil)
}

func (d *DongleVirtual) macAuth(t protocolo.Tipo, payload []byte) []byte {
	m := hmac.New(sha256.New, d.ch.KAuth)
	m.Write(protocolo.EntradaTag(t, d.ch.Dev, payload))
	return m.Sum(nil)[:protocolo.TamTagAuth]
}

// boot (re)inicia o dongle: perdidos = blocos que só estavam na RAM e se perderam.
func (d *DongleVirtual) boot(perdidos uint32) {
	d.bootID++
	if d.bootID == 0 {
		d.bootID = 1
	}
	d.inicio = time.Now()
	from := d.seq + 1
	d.seq += perdidos + 1
	dados := protocolo.MontarMarcador(d.seq, d.bootID, 0, protocolo.RegBoot, from, d.seq-1)
	d.empurrar(blocoFlash{seq: d.seq, boot: d.bootID, boot_: true, from: from,
		tlv: protocolo.MontarBlocoTLV(dados, d.assinar(dados))})
}

func (d *DongleVirtual) empurrar(b blocoFlash) {
	d.flash = append(d.flash, b)
	for len(d.flash) > d.cfg.CapacidadeFlash {
		d.flash = d.flash[1:] // descarta o mais antigo
	}
}

// PerderEnergia simula a queda: perdidos blocos da RAM somem e um novo boot começa.
func (d *DongleVirtual) PerderEnergia(perdidos uint32) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.boot(perdidos)
}

// GerarBlocos produz n blocos de dados (PIDs + GPS) assinados com K_rec.
func (d *DongleVirtual) GerarBlocos(n int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := 0; i < n; i++ {
		d.seq++
		d.n++
		t0 := d.tMs(time.Now())
		m := protocolo.NovoMontadorBloco(d.seq, d.bootID, t0)
		regs := 0
		for j := 0; j < 20; j++ {
			dt := uint16(j * 40)
			m.PID(12, dt, 0x17, byte(d.n+j))
			m.PID(13, dt, byte(40+j), 0)
			m.PID(4, dt, byte(30+j), 0)
			regs += 3
		}
		m.GPS(100, -235505200+int32(d.n)*10, -466333100, 760000)
		regs++
		dados := m.Dados()
		d.Registros[d.seq] = regs
		d.Gerados += regs
		d.empurrar(blocoFlash{seq: d.seq, boot: d.bootID, registro: regs,
			tlv: protocolo.MontarBlocoTLV(dados, d.assinar(dados))})
	}
}

// Corromper marca o bloco como ilegível (CRC): vira LOSS no envio.
func (d *DongleVirtual) Corromper(seq uint32) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := range d.flash {
		if d.flash[i].seq == seq {
			d.flash[i].corrompido = true
			d.Gerados -= d.flash[i].registro
		}
	}
}

// Head/Tail/Ack.
func (d *DongleVirtual) Head() uint32 { d.mu.Lock(); defer d.mu.Unlock(); return d.seq }

func (d *DongleVirtual) tail() uint32 {
	if len(d.flash) == 0 {
		return d.seq + 1
	}
	o := d.flash[0]
	if o.boot_ && o.from < o.seq {
		return o.from
	}
	return o.seq
}

// AckLocal é o último ACK gravado.
func (d *DongleVirtual) AckLocal() uint32 { d.mu.Lock(); defer d.mu.Unlock(); return d.ackLocal }

func (d *DongleVirtual) liberar(ack uint32) {
	if ack > d.ackLocal {
		d.ackLocal = ack
	}
	i := 0
	for i < len(d.flash) && d.flash[i].seq <= d.ackLocal {
		i++
	}
	d.flash = d.flash[i:]
}

// ErrDongle: mensagem rejeitada pelo dongle (Tabela 5).
var ErrDongle = errors.New("dongle recusou a mensagem")

// ProcessarDesafio verifica o Desafio 0x01 e devolve a Resposta 0x02 (TLV completo).
func (d *DongleVirtual) ProcessarDesafio(tlv []byte) ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	t, n, err := protocolo.LerTLV(tlv)
	if err != nil || n != len(tlv) || t.T != protocolo.TipoDesafio {
		return nil, ErrDongle
	}
	des, tag, err := protocolo.DecodificarDesafio(t.V)
	if err != nil {
		return nil, err
	}
	if !hmac.Equal(d.macAuth(protocolo.TipoDesafio, des.Payload()), tag) {
		return nil, ErrDongle
	}
	if des.CS <= d.ultimoCS {
		return nil, ErrDongle
	}
	d.ultimoCS = des.CS
	d.liberar(des.AckSeq)
	r := protocolo.Resposta{CS: des.CS, NS: des.NS, TailSeq: d.tail(), HeadSeq: d.seq, BootID: d.bootID, TMs: d.tMs(time.Now())}
	return r.Montar(d.macAuth(protocolo.TipoResposta, r.Payload())), nil
}

// ProcessarAck verifica o ACK 0x04, libera a flash e devolve se a âncora falhou.
func (d *DongleVirtual) ProcessarAck(tlv []byte) (ancoraFalhou bool, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	t, n, err := protocolo.LerTLV(tlv)
	if err != nil || n != len(tlv) || t.T != protocolo.TipoAck {
		return false, ErrDongle
	}
	a, tag, err := protocolo.DecodificarAck(t.V)
	if err != nil {
		return false, err
	}
	if !hmac.Equal(d.macAuth(protocolo.TipoAck, a.Payload()), tag) {
		return false, ErrDongle
	}
	d.liberar(a.AckSeq)
	return a.Flags&protocolo.FlagAckAncoraFalhou != 0, nil
}

// LerParaEnvio devolve até k TLVs a partir de ACK+1; bloco corrompido vira LOSS (um BOOT corrompido
// vira LOSS em pedaços: [prev+1, seq-1] e [seq, seq]).
func (d *DongleVirtual) LerParaEnvio(k int) [][]byte {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out [][]byte
	prev := d.ackLocal
	loss := func(seq, from, to uint32) []byte {
		dados := protocolo.MontarMarcador(seq, 0, 0, protocolo.RegLoss, from, to)
		return protocolo.MontarBlocoTLV(dados, d.assinar(dados))
	}
	for _, b := range d.flash {
		if b.seq <= d.ackLocal {
			continue
		}
		if len(out) >= k {
			break
		}
		switch {
		case b.corrompido && b.boot_:
			if b.seq > prev+1 {
				out = append(out, loss(prev+1, prev+1, b.seq-1))
			}
			out = append(out, loss(b.seq, b.seq, b.seq))
		case b.corrompido:
			out = append(out, loss(b.seq, b.seq, b.seq))
		default:
			out = append(out, b.tlv)
		}
		prev = b.seq
	}
	return out
}

// RespostaForjada devolve um 0x02 qualquer com tag errada (testes negativos).
func (d *DongleVirtual) NovaConexaoBLE() {}

// NonceAleatorio só para testes.
func NonceAleatorio() [16]byte {
	var n [16]byte
	_, _ = rand.Read(n[:])
	return n
}

// DescartarSegmento remove os n blocos mais antigos da flash (descarte por segmento do firmware).
func (d *DongleVirtual) DescartarSegmento(n int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if n > len(d.flash) {
		n = len(d.flash)
	}
	d.flash = d.flash[n:]
}

// UltimoSeqBoot devolve o seq do BOOT mais recente na flash (0 se não houver).
func (d *DongleVirtual) UltimoSeqBoot() uint32 {
	d.mu.Lock()
	defer d.mu.Unlock()
	var s uint32
	for _, b := range d.flash {
		if b.boot_ {
			s = b.seq
		}
	}
	return s
}
