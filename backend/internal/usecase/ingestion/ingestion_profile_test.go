package ingestion

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"testing"

	"tccelta/backend/internal/domain/dongle"
	ding "tccelta/backend/internal/domain/ingestion"
	"tccelta/backend/internal/domain/protocol"
)

const (
	asymID = 11
	noneID = 12
)

var asymDev = protocol.DevID{0, 0xA5, 0, 0, 0, 0, 0, 1}

// asymKey cadastra um dongle asym com a chave pública de um par novo e o aponta como sessão.
func asymKey(t *testing.T, a *testEnv) *ecdsa.PrivateKey {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pub := append([]byte{0x04}, append(k.X.FillBytes(make([]byte, 32)), k.Y.FillBytes(make([]byte, 32))...)...)
	a.db.AddDongle(dongle.Dongle{ID: asymID, DevID: asymDev, Profile: protocol.ProfileAsym, AuthKey: "ka", RecKey: "kr",
		PublicKey: pub, Active: true})
	a.session.Dev = asymDev
	return k
}

func (b block) data() []byte {
	m := protocol.NewBlockBuilder(b.seq, b.boot, 1000)
	m.PID(12, 0, 0x17, 0x70)
	return m.Data()
}

// asymTLV assina o bloco com ECDSA (r‖s); bad troca 1 bit do dado já assinado.
func asymTLV(k *ecdsa.PrivateKey, b block) []byte {
	data := b.data()
	h := sha256.Sum256(protocol.TagInput(protocol.BlockType, asymDev, data))
	r, s, err := ecdsa.Sign(rand.Reader, k, h[:])
	if err != nil {
		panic(err)
	}
	tag := append(r.FillBytes(make([]byte, 32)), s.FillBytes(make([]byte, 32))...)
	if b.bad {
		tag[10] ^= 1
	}
	return protocol.BuildBlockTLV(data, tag)
}

func asymBatch(k *ecdsa.PrivateKey, bs ...block) []byte {
	var out []byte
	for _, b := range bs {
		out = append(out, asymTLV(k, b)...)
	}
	return out
}

func TestAsymBatchAcceptedWithoutKMSVerify(t *testing.T) {
	a := newTest(t)
	k := asymKey(t, a)
	r, err := a.send(t, asymBatch(k, seqs(1, 3)...))
	if err != nil {
		t.Fatal(err)
	}
	if r.Accepted != 3 || r.AckSeq != 3 || r.Interrupted {
		t.Fatalf("%+v", r)
	}
	if n := a.kms.VerifyCalls.Load(); n != 0 {
		t.Fatalf("asym não pode chamar VerifyBlocks (chamado %d vezes)", n)
	}
	if n := a.kms.HMACCalls.Load(); n != 1 {
		t.Fatalf("o ACK segue HMAC K_auth: %d chamadas, esperado 1", n)
	}
	tl, _, _ := protocol.ReadTLV(r.AckTLV)
	_, tag, _ := protocol.DecodeAck(tl.V)
	ack := ackFrom(t, r.AckTLV)
	m := hmac.New(sha256.New, kAuth)
	m.Write(protocol.TagInput(protocol.AckType, asymDev, ack.Payload()))
	if !protocol.CompareTag(m.Sum(nil)[:protocol.AuthTagSize], tag) {
		t.Fatal("tag do ACK não confere")
	}
}

func TestAsymTamperedTagIsInvalidTag(t *testing.T) {
	a := newTest(t)
	k := asymKey(t, a)
	r, err := a.send(t, asymBatch(k, block{seq: 1, boot: 1}, block{seq: 2, boot: 1, bad: true}, block{seq: 3, boot: 1}))
	if err != nil {
		t.Fatal(err)
	}
	if r.AckSeq != 1 || r.Reason != ding.ReasonInvalidTag || !r.Interrupted {
		t.Fatalf("%+v", r)
	}
	if len(a.db.Rejected) != 1 || a.db.Rejected[0].Seq != 2 || a.db.Rejected[0].Reason != ding.ReasonInvalidTag {
		t.Fatalf("%+v", a.db.Rejected)
	}
	if a.kms.VerifyCalls.Load() != 0 {
		t.Fatal("asym não chama o KMS para verificar")
	}
}

func TestAsymFirstBlockInvalidMakesNoProgress(t *testing.T) {
	a := newTest(t)
	k := asymKey(t, a)
	r, err := a.send(t, asymBatch(k, block{seq: 1, boot: 1, bad: true}))
	if err != nil {
		t.Fatal(err)
	}
	if !r.NoProgress || r.AckSeq != 0 {
		t.Fatalf("%+v", r)
	}
}

func TestAsymWrongKeyIsInvalid(t *testing.T) {
	a := newTest(t)
	asymKey(t, a)
	other, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	r, err := a.send(t, asymBatch(other, seqs(1, 2)...))
	if err != nil {
		t.Fatal(err)
	}
	if r.Reason != ding.ReasonInvalidTag || r.Accepted != 0 {
		t.Fatalf("%+v", r)
	}
}

func TestAsymInvalidPublicKeyMakesEverythingInvalid(t *testing.T) {
	a := newTest(t)
	k := asymKey(t, a)
	a.db.AddDongle(dongle.Dongle{ID: asymID, DevID: asymDev, Profile: protocol.ProfileAsym, AuthKey: "ka", RecKey: "kr",
		PublicKey: []byte{0x04, 1, 2, 3}, Active: true})
	r, err := a.send(t, asymBatch(k, seqs(1, 2)...))
	if err != nil {
		t.Fatalf("chave inválida não pode virar erro: %v", err)
	}
	if r.Reason != ding.ReasonInvalidTag || r.Accepted != 0 || !r.NoProgress {
		t.Fatalf("%+v", r)
	}
}

func TestAsymDuplicateSeqInheritsFirstResult(t *testing.T) {
	a := newTest(t)
	k := asymKey(t, a)
	r, err := a.send(t, asymBatch(k, block{seq: 1, boot: 1}, block{seq: 1, boot: 1, bad: true}))
	if err != nil {
		t.Fatal(err)
	}
	if r.Accepted != 1 || r.AckSeq != 1 || len(a.db.Rejected) != 0 {
		t.Fatalf("%+v rejeitados=%+v", r, a.db.Rejected)
	}
}

func TestNoneEmptyTagAccepted(t *testing.T) {
	a := newTest(t)
	a.db.AddDongle(dongle.Dongle{ID: noneID, DevID: protocol.DevID{0x0E}, Profile: protocol.ProfileNone, AuthKey: "ka", Active: true})
	a.session.Dev = protocol.DevID{0x0E}
	a.receive.p.AllowNoneProfile = true
	var body []byte
	for _, b := range seqs(1, 2) {
		body = append(body, protocol.BuildBlockTLV(b.data(), nil)...)
	}
	r, err := a.send(t, body)
	if err != nil {
		t.Fatal(err)
	}
	if r.Accepted != 2 || r.AckSeq != 2 || a.kms.VerifyCalls.Load() != 0 {
		t.Fatalf("%+v verify=%d", r, a.kms.VerifyCalls.Load())
	}
}
