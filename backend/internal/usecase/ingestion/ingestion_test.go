package ingestion

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"tccelta/backend/internal/adapter/memory"
	"tccelta/backend/internal/domain/dongle"
	ding "tccelta/backend/internal/domain/ingestion"
	"tccelta/backend/internal/domain/occurrence"
	"tccelta/backend/internal/domain/pid"
	"tccelta/backend/internal/domain/protocol"
	"tccelta/backend/internal/port"
	"tccelta/backend/internal/port/porttest"
)

var (
	dev   = protocol.DevID{0, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77}
	kAuth = []byte("0123456789abcdef0123456789abcdef")
	kRec  = []byte("fedcba9876543210fedcba9876543210")
	t0    = time.Unix(1_800_000_000, 0)
)

func f(v float64) *float64 { return &v }

type testEnv struct {
	db      *memory.Database
	kms     *porttest.Memory
	receive *ReceiveBatch
	conf    *GetConfirmation
	session port.Session
}

func newTest(t *testing.T) *testEnv {
	t.Helper()
	db := memory.New()
	db.AddDongle(dongle.Dongle{ID: 7, DevID: dev, Profile: protocol.ProfileHMAC, AuthKey: "ka", RecKey: "kr", Active: true})
	k := porttest.New()
	k.AddDongle("ka", "kr", kAuth, kRec)
	cat := pid.FromMap([]pid.Def{
		{PID: 4, NBytes: 1, Factor: 100.0 / 255, Min: f(0), Max: f(100)},
		{PID: 12, NBytes: 2, Factor: 0.25, Min: f(0), Max: f(16383.75)},
	})
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	p := Params{MaxBatch: 32, VerifyBatch: 2, VerifyParallel: 2}
	repos := db.Repositories()
	return &testEnv{db: db, kms: k,
		receive: NewReceiveBatch(repos, db, k, cat, porttest.Nop{}, porttest.FixedClock(t0), log, p),
		conf:    NewGetConfirmation(repos, false),
		session: port.Session{Dev: dev, CS: 3, JTI: uuid.New(), ExpiresAt: t0.Add(time.Hour)}}
}

func mac(key []byte, typ protocol.Type, payload []byte) []byte {
	m := hmac.New(sha256.New, key)
	m.Write(protocol.TagInput(typ, dev, payload))
	return m.Sum(nil)
}

// block monta o TLV de um bloco de dados (PID 12) com tag K_rec; ruim corrompe a tag.
type block struct {
	seq  uint32
	boot uint16
	pid  uint8 // 0 => 12
	bad  bool
	raw  []byte // dados prontos (marcadores)
}

func (b block) tlv() []byte {
	data := b.raw
	if data == nil {
		m := protocol.NewBlockBuilder(b.seq, b.boot, 1000)
		p := b.pid
		if p == 0 {
			p = 12
		}
		m.PID(p, 0, 0x17, 0x70)
		m.GPS(10, -235505200, -466333100, 760000)
		data = m.Data()
	}
	tag := mac(kRec, protocol.BlockType, data)
	if b.bad {
		tag[0] ^= 1
	}
	return protocol.BuildBlockTLV(data, tag)
}

func batch(bs ...block) []byte {
	var out []byte
	for _, b := range bs {
		out = append(out, b.tlv()...)
	}
	return out
}

func seqs(a, b uint32) []block {
	var o []block
	for s := a; s <= b; s++ {
		o = append(o, block{seq: s, boot: 1})
	}
	return o
}

func (a *testEnv) send(t *testing.T, body []byte) (BatchResult, error) {
	t.Helper()
	return a.receive.Run(context.Background(), ReceiveBatchInput{Session: a.session, Body: body})
}

func ackFrom(t *testing.T, tlv []byte) protocol.Ack {
	t.Helper()
	a, ok := ackDecoded(tlv)
	if !ok {
		t.Fatal("ACK inválido")
	}
	return a
}

func TestFullBatch(t *testing.T) {
	a := newTest(t)
	r, err := a.send(t, batch(seqs(1, 3)...))
	if err != nil {
		t.Fatal(err)
	}
	if r.Accepted != 3 || r.AckSeq != 3 || r.Interrupted || r.Action != ding.ActionNone {
		t.Fatalf("%+v", r)
	}
	if ack := ackFrom(t, r.AckTLV); ack.AckSeq != 3 || ack.CConf != 0 {
		t.Fatalf("%+v", ack)
	}
	tl, _, _ := protocol.ReadTLV(r.AckTLV)
	_, tag, _ := protocol.DecodeAck(tl.V)
	ack := ackFrom(t, r.AckTLV)
	if !protocol.CompareTag(mac(kAuth, protocol.AckType, ack.Payload())[:protocol.AuthTagSize], tag) {
		t.Fatal("tag do ACK não confere")
	}
	if len(a.db.RawBlocks) != 3 || len(a.db.Readings) != 3 || len(a.db.Positions) != 3 {
		t.Fatalf("brutos=%d leituras=%d posicoes=%d", len(a.db.RawBlocks), len(a.db.Readings), len(a.db.Positions))
	}
	if est := a.db.State(7); est.ContiguousSeq != 3 || !bytes.Equal(est.LastAck, r.AckTLV) {
		t.Fatalf("%+v", est)
	}
	// ACK provável acertado: uma única chamada de HMAC (pré-assinatura reaproveitada na transação)
	if n := a.kms.HMACCalls.Load(); n != 1 {
		t.Fatalf("HMAC chamado %d vezes, esperado 1", n)
	}
	// o verify foi em lotes de 2: 3 blocos => 2 chamadas
	if n := a.kms.VerifyCalls.Load(); n != 2 {
		t.Fatalf("verify chamado %d vezes, esperado 2", n)
	}
}

func TestDuplicateResendReusesAck(t *testing.T) {
	a := newTest(t)
	body := batch(seqs(1, 3)...)
	r1, _ := a.send(t, body)
	hmacs, verifies := a.kms.HMACCalls.Load(), a.kms.VerifyCalls.Load()
	r2, err := a.send(t, body)
	if err != nil {
		t.Fatal(err)
	}
	if r2.Accepted != 0 || r2.Duplicates != 3 || !bytes.Equal(r2.AckTLV, r1.AckTLV) {
		t.Fatalf("%+v", r2)
	}
	if a.kms.HMACCalls.Load() != hmacs || a.kms.VerifyCalls.Load() != verifies {
		t.Fatal("duplicados não devem chamar o KMS")
	}
	if len(a.db.RawBlocks) != 3 {
		t.Fatal("nada novo para gravar")
	}
}

func TestJumpStops(t *testing.T) {
	a := newTest(t)
	r, err := a.send(t, batch(block{seq: 1, boot: 1}, block{seq: 2, boot: 1}, block{seq: 4, boot: 1}))
	if err != nil {
		t.Fatal(err)
	}
	if r.AckSeq != 2 || !r.Interrupted || r.Reason != ding.ReasonJump || r.Action != ding.ActionReread || r.NoProgress {
		t.Fatalf("%+v", r)
	}
}

func TestInvalidTagRejectsAndStops(t *testing.T) {
	a := newTest(t)
	r, err := a.send(t, batch(block{seq: 1, boot: 1}, block{seq: 2, boot: 1, bad: true}, block{seq: 3, boot: 1}))
	if err != nil {
		t.Fatal(err)
	}
	if r.AckSeq != 1 || r.Reason != ding.ReasonInvalidTag || r.NoProgress || !r.Interrupted {
		t.Fatalf("%+v", r)
	}
	if len(a.db.Rejected) != 1 || a.db.Rejected[0].Reason != ding.ReasonInvalidTag || a.db.Rejected[0].Seq != 2 ||
		a.db.Rejected[0].TokenJTI != a.session.JTI {
		t.Fatalf("%+v", a.db.Rejected)
	}
	// a previsão do ACK (tudo válido) errou: HMAC extra dentro da transação
	if n := a.kms.HMACCalls.Load(); n != 2 {
		t.Fatalf("HMAC chamado %d vezes, esperado 2 (pré + tx)", n)
	}
}

func TestFirstBlockWithInvalidTagMakesNoProgress(t *testing.T) {
	a := newTest(t)
	r, err := a.send(t, batch(block{seq: 1, boot: 1, bad: true}))
	if err != nil {
		t.Fatal(err)
	}
	if !r.NoProgress || r.AckSeq != 0 {
		t.Fatalf("%+v", r)
	}
}

func TestUnknownPID(t *testing.T) {
	a := newTest(t)
	r, err := a.send(t, batch(block{seq: 1, boot: 1}, block{seq: 2, boot: 1, pid: 0x99}))
	if err != nil {
		t.Fatal(err)
	}
	if r.AckSeq != 1 || r.Reason != ding.ReasonUnknownPID || r.Action != ding.ActionWait {
		t.Fatalf("%+v", r)
	}
	if len(a.db.Rejected) != 1 || len(a.db.Alerts) != 1 || a.db.Alerts[0].Type != occurrence.AlertUnknownPID {
		t.Fatalf("%+v %+v", a.db.Rejected, a.db.Alerts)
	}
}

func TestLossAndBootMarkers(t *testing.T) {
	a := newTest(t)
	loss := block{raw: protocol.BuildMarker(3, 1, 0, protocol.RegLoss, 2, 3)}
	r, err := a.send(t, batch(block{seq: 1, boot: 1}, loss))
	if err != nil {
		t.Fatal(err)
	}
	if r.AckSeq != 3 || len(a.db.Gaps) != 1 || a.db.Gaps[0].Cause != occurrence.CauseLoss ||
		a.db.Gaps[0].From != 2 || a.db.Gaps[0].To != 3 {
		t.Fatalf("%+v %+v", r, a.db.Gaps)
	}
	boot := block{raw: protocol.BuildMarker(8, 2, 0, protocol.RegBoot, 4, 7)}
	r, err = a.send(t, batch(boot, block{seq: 9, boot: 2}))
	if err != nil {
		t.Fatal(err)
	}
	if r.AckSeq != 9 || len(a.db.Gaps) != 2 || a.db.Gaps[1].Cause != occurrence.CauseBoot ||
		a.db.Gaps[1].From != 4 || a.db.Gaps[1].To != 7 {
		t.Fatalf("%+v %+v", r, a.db.Gaps)
	}
}

func TestGenerationAdvances(t *testing.T) {
	a := newTest(t)
	a.db.SetState(7, dongle.State{ContiguousSeq: 4, MaxSeenSeq: 4, LastBootID: 65535})
	if _, err := a.send(t, batch(block{seq: 5, boot: 1})); err != nil { // boot_id voltou: nova geração
		t.Fatal(err)
	}
	if est := a.db.State(7); est.BootGeneration != 1 || est.LastBootID != 1 {
		t.Fatalf("%+v", est)
	}
	if len(a.db.Alerts) != 1 || a.db.Alerts[0].Type != occurrence.AlertBootGenerationAdvanced {
		t.Fatalf("%+v", a.db.Alerts)
	}
}

func TestMalformedAndLargeBatch(t *testing.T) {
	a := newTest(t)
	if _, err := a.send(t, []byte{0xFF, 0x00}); !errors.Is(err, ding.ErrMalformedBatch) {
		t.Fatal(err)
	}
	var bs []block
	for s := uint32(1); s <= 40; s++ {
		bs = append(bs, block{seq: s, boot: 1})
	}
	if _, err := a.send(t, batch(bs...)); !errors.Is(err, ding.ErrBatchTooLarge) {
		t.Fatal(err)
	}
}

func TestKMSUnavailableWritesNothing(t *testing.T) {
	a := newTest(t)
	failure := port.ErrKMSUnavailable
	a.kms.Failure.Store(&failure)
	if _, err := a.send(t, batch(seqs(1, 2)...)); !errors.Is(err, port.ErrKMSUnavailable) {
		t.Fatal(err)
	}
	if len(a.db.RawBlocks) != 0 || a.db.State(7).ContiguousSeq != 0 {
		t.Fatal("nada pode ser gravado")
	}
}

func TestGetConfirmation(t *testing.T) {
	a := newTest(t)
	if _, err := a.conf.Run(context.Background(), GetConfirmationInput{DevID: dev}); !errors.Is(err, ding.ErrNoConfirmation) {
		t.Fatal(err)
	}
	r, _ := a.send(t, batch(seqs(1, 1)...))
	ack, err := a.conf.Run(context.Background(), GetConfirmationInput{DevID: dev})
	if err != nil || !bytes.Equal(ack, r.AckTLV) {
		t.Fatalf("%v %x", err, ack)
	}
	n := a.kms.HMACCalls.Load()
	_, _ = a.conf.Run(context.Background(), GetConfirmationInput{DevID: dev})
	if a.kms.HMACCalls.Load() != n {
		t.Fatal("a confirmação não chama o KMS")
	}
}

func TestAccessDenied(t *testing.T) {
	a := newTest(t)
	a.db.AddDongle(dongle.Dongle{ID: 8, DevID: protocol.DevID{8}, Profile: protocol.ProfileNone, Active: true})
	a.session.Dev = protocol.DevID{8}
	if _, err := a.send(t, batch(seqs(1, 1)...)); !errors.Is(err, dongle.ErrProfileNotAllowed) {
		t.Fatal(err)
	}
	a.db.AddDongle(dongle.Dongle{ID: 9, DevID: protocol.DevID{9}, Profile: protocol.ProfileHMAC, Active: false})
	a.session.Dev = protocol.DevID{9}
	if _, err := a.send(t, batch(seqs(1, 1)...)); !errors.Is(err, dongle.ErrInactive) {
		t.Fatal(err)
	}
}
