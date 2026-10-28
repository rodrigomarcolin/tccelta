// Package simulator contém dongles e apps virtuais que usam o mesmo pacote protocolo da API.
package simulator

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"sync"
	"time"

	"tccelta/backend/internal/domain/protocol"
)

// Keys de um dongle virtual (as mesmas importadas no Vault de teste).
type Keys struct {
	Dev         protocol.DevID
	KAuth, KRec []byte
}

// ConfigDongle parametriza o comportamento do firmware simulado.
type ConfigDongle struct {
	FlashCapacity int     // blocos
	DriftPPM      float64 // deriva do cristal
	InitialBoot   uint16  // boot_id do primeiro boot
}

type flashBlock struct {
	seq       uint32
	boot      uint16
	tlv       []byte
	record    int // nº de registros de PID+GPS (0 em marcador)
	boot_     bool
	from      uint32
	corrupted bool
}

// DongleVirtual reproduz o lado dongle do protocolo (spec 10).
type DongleVirtual struct {
	mu       sync.Mutex
	ch       Keys
	cfg      ConfigDongle
	bootID   uint16
	seq      uint32 // último seq atribuído
	ackLocal uint32
	lastCS   uint32
	start    time.Time
	flash    []flashBlock
	// Records por seq (para as verificações).
	Records   map[uint32]int
	Generated int
	n         int
}

// NewDongle cria um dongle e executa o primeiro boot (gera o BOOT de faixa vazia).
func NewDongle(ch Keys, cfg ConfigDongle) *DongleVirtual {
	if cfg.FlashCapacity <= 0 {
		cfg.FlashCapacity = 1000000
	}
	if cfg.InitialBoot == 0 {
		cfg.InitialBoot = 1
	}
	d := &DongleVirtual{ch: ch, cfg: cfg, bootID: cfg.InitialBoot - 1, Records: map[uint32]int{}}
	d.boot(0)
	return d
}

func (d *DongleVirtual) tMs(t time.Time) uint32 {
	ms := float64(t.Sub(d.start).Milliseconds()) * (1 + d.cfg.DriftPPM*1e-6)
	return uint32(int64(ms))
}

func (d *DongleVirtual) sign(data []byte) []byte {
	m := hmac.New(sha256.New, d.ch.KRec)
	m.Write(protocol.TagInput(protocol.BlockType, d.ch.Dev, data))
	return m.Sum(nil)
}

func (d *DongleVirtual) macAuth(t protocol.Type, payload []byte) []byte {
	m := hmac.New(sha256.New, d.ch.KAuth)
	m.Write(protocol.TagInput(t, d.ch.Dev, payload))
	return m.Sum(nil)[:protocol.AuthTagSize]
}

// boot (re)inicia o dongle: perdidos = blocos que só estavam na RAM e se perderam.
func (d *DongleVirtual) boot(lost uint32) {
	d.bootID++
	if d.bootID == 0 {
		d.bootID = 1
	}
	d.start = time.Now()
	from := d.seq + 1
	d.seq += lost + 1
	data := protocol.BuildMarker(d.seq, d.bootID, 0, protocol.RegBoot, from, d.seq-1)
	d.push(flashBlock{seq: d.seq, boot: d.bootID, boot_: true, from: from,
		tlv: protocol.BuildBlockTLV(data, d.sign(data))})
}

func (d *DongleVirtual) push(b flashBlock) {
	d.flash = append(d.flash, b)
	for len(d.flash) > d.cfg.FlashCapacity {
		d.flash = d.flash[1:] // descarta o mais antigo
	}
}

// LosePower simula a queda: perdidos blocos da RAM somem e um novo boot começa.
func (d *DongleVirtual) LosePower(lost uint32) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.boot(lost)
}

// GenerateBlocks produz n blocos de dados (PIDs + GPS) assinados com K_rec.
func (d *DongleVirtual) GenerateBlocks(n int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := 0; i < n; i++ {
		d.seq++
		d.n++
		t0 := d.tMs(time.Now())
		m := protocol.NewBlockBuilder(d.seq, d.bootID, t0)
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
		data := m.Data()
		d.Records[d.seq] = regs
		d.Generated += regs
		d.push(flashBlock{seq: d.seq, boot: d.bootID, record: regs,
			tlv: protocol.BuildBlockTLV(data, d.sign(data))})
	}
}

// Corrupt marca o bloco como ilegível (CRC): vira LOSS no envio.
func (d *DongleVirtual) Corrupt(seq uint32) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := range d.flash {
		if d.flash[i].seq == seq {
			d.flash[i].corrupted = true
			d.Generated -= d.flash[i].record
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

func (d *DongleVirtual) release(ack uint32) {
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

// ProcessChallenge verifica o Desafio 0x01 e devolve a Resposta 0x02 (TLV completo).
func (d *DongleVirtual) ProcessChallenge(tlv []byte) ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	t, n, err := protocol.ReadTLV(tlv)
	if err != nil || n != len(tlv) || t.T != protocol.ChallengeType {
		return nil, ErrDongle
	}
	ch, tag, err := protocol.DecodeChallenge(t.V)
	if err != nil {
		return nil, err
	}
	if !hmac.Equal(d.macAuth(protocol.ChallengeType, ch.Payload()), tag) {
		return nil, ErrDongle
	}
	if ch.CS <= d.lastCS {
		return nil, ErrDongle
	}
	d.lastCS = ch.CS
	d.release(ch.AckSeq)
	r := protocol.Response{CS: ch.CS, NS: ch.NS, TailSeq: d.tail(), HeadSeq: d.seq, BootID: d.bootID, TMs: d.tMs(time.Now())}
	return r.Build(d.macAuth(protocol.ResponseType, r.Payload())), nil
}

// ProcessAck verifica o ACK 0x04, libera a flash e devolve se a âncora falhou.
func (d *DongleVirtual) ProcessAck(tlv []byte) (anchorFailed bool, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	t, n, err := protocol.ReadTLV(tlv)
	if err != nil || n != len(tlv) || t.T != protocol.AckType {
		return false, ErrDongle
	}
	a, tag, err := protocol.DecodeAck(t.V)
	if err != nil {
		return false, err
	}
	if !hmac.Equal(d.macAuth(protocol.AckType, a.Payload()), tag) {
		return false, ErrDongle
	}
	d.release(a.AckSeq)
	return a.Flags&protocol.FlagAckAnchorFailed != 0, nil
}

// ReadForSending devolve até k TLVs a partir de ACK+1; bloco corrompido vira LOSS (um BOOT corrompido
// vira LOSS em pedaços: [prev+1, seq-1] e [seq, seq]).
func (d *DongleVirtual) ReadForSending(k int) [][]byte {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out [][]byte
	prev := d.ackLocal
	loss := func(seq, from, to uint32) []byte {
		data := protocol.BuildMarker(seq, 0, 0, protocol.RegLoss, from, to)
		return protocol.BuildBlockTLV(data, d.sign(data))
	}
	for _, b := range d.flash {
		if b.seq <= d.ackLocal {
			continue
		}
		if len(out) >= k {
			break
		}
		switch {
		case b.corrupted && b.boot_:
			if b.seq > prev+1 {
				out = append(out, loss(prev+1, prev+1, b.seq-1))
			}
			out = append(out, loss(b.seq, b.seq, b.seq))
		case b.corrupted:
			out = append(out, loss(b.seq, b.seq, b.seq))
		default:
			out = append(out, b.tlv)
		}
		prev = b.seq
	}
	return out
}

// RespostaForjada devolve um 0x02 qualquer com tag errada (testes negativos).
func (d *DongleVirtual) NewBLEConnection() {}

// RandomNonce só para testes.
func RandomNonce() [16]byte {
	var n [16]byte
	_, _ = rand.Read(n[:])
	return n
}

// DiscardSegment remove os n blocos mais antigos da flash (descarte por segmento do firmware).
func (d *DongleVirtual) DiscardSegment(n int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if n > len(d.flash) {
		n = len(d.flash)
	}
	d.flash = d.flash[n:]
}

// LastSeqBoot devolve o seq do BOOT mais recente na flash (0 se não houver).
func (d *DongleVirtual) LastSeqBoot() uint32 {
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
