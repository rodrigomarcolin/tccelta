package protocol

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func macSHA(key, msg []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(msg)
	return h.Sum(nil)
}

type vectors struct {
	DevID    string `json:"dev_id"`
	KAuth    string `json:"k_auth"`
	KRec     string `json:"k_rec"`
	Usecases []struct {
		Name      string          `json:"nome"`
		Type      int             `json:"tipo"`
		Fields    json.RawMessage `json:"campos"`
		HMACInput string          `json:"entrada_hmac"`
		HMACCompl string          `json:"hmac_completo"`
		Data      string          `json:"dados"`
		Tag       string          `json:"tag"`
		TLV       string          `json:"tlv"`
	} `json:"casos"`
}

func TestPythonVectors(t *testing.T) {
	raw, err := os.ReadFile("testdata/vetores.json")
	if err != nil {
		t.Fatal(err)
	}
	var v vectors
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	dev, err := DevIDFromHex(v.DevID)
	if err != nil {
		t.Fatal(err)
	}
	kAuth, kRec := mustHex(t, v.KAuth), mustHex(t, v.KRec)
	for _, c := range v.Usecases {
		t.Run(c.Name, func(t *testing.T) {
			tl, n, err := ReadTLV(mustHex(t, c.TLV))
			if err != nil || n != len(mustHex(t, c.TLV)) {
				t.Fatalf("LerTLV: %v", err)
			}
			if int(tl.T) != c.Type {
				t.Fatalf("tipo %d", tl.T)
			}
			switch tl.T {
			case ChallengeType:
				d, tag, err := DecodeChallenge(tl.V)
				if err != nil {
					t.Fatal(err)
				}
				e := TagInput(ChallengeType, dev, d.Payload())
				if hex.EncodeToString(e) != c.HMACInput {
					t.Fatalf("entrada\n%x\n%s", e, c.HMACInput)
				}
				if !CompareTag(macSHA(kAuth, e)[:16], tag) {
					t.Fatal("tag")
				}
				re, err := d.Build(tag)
				if err != nil || !bytes.Equal(re, mustHex(t, c.TLV)) {
					t.Fatalf("remontar: %v", err)
				}
			case ResponseType:
				r, p, tag, err := DecodeResponse(tl.V)
				if err != nil {
					t.Fatal(err)
				}
				e := TagInput(ResponseType, dev, p)
				if hex.EncodeToString(e) != c.HMACInput || !CompareTag(macSHA(kAuth, e)[:16], tag) {
					t.Fatal("entrada/tag")
				}
				if !bytes.Equal(r.Build(tag), mustHex(t, c.TLV)) {
					t.Fatal("remontar")
				}
			case AckType:
				a, tag, err := DecodeAck(tl.V)
				if err != nil {
					t.Fatal(err)
				}
				e := TagInput(AckType, dev, a.Payload())
				if hex.EncodeToString(e) != c.HMACInput || !CompareTag(macSHA(kAuth, e)[:16], tag) {
					t.Fatal("entrada/tag")
				}
				if !bytes.Equal(a.Build(tag), mustHex(t, c.TLV)) {
					t.Fatal("remontar")
				}
			case BlockType:
				b, err := DecodeBlock(tl.V, ProfileHMAC)
				if err != nil {
					t.Fatal(err)
				}
				if hex.EncodeToString(b.Data) != c.Data || hex.EncodeToString(b.Tag) != c.Tag {
					t.Fatal("dados/tag")
				}
				e := TagInput(BlockType, dev, b.Data)
				if hex.EncodeToString(e) != c.HMACInput || !CompareTag(macSHA(kRec, e), b.Tag) {
					t.Fatal("entrada/tag")
				}
				if _, err := ReadRecords(b.Data); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

type firmwareVectors struct {
	DevID     string `json:"dev_id"`
	KAuth     string `json:"k_auth"`
	KRec      string `json:"k_rec"`
	Handshake []struct {
		Name    string `json:"name"`
		Type    int    `json:"type"`
		Payload string `json:"payload"`
		Tag     string `json:"tag"`
	} `json:"handshake"`
	Blocks []struct {
		Name string `json:"name"`
		Data string `json:"data"`
		Tag  string `json:"tag"`
	} `json:"blocks"`
}

func TestFirmwareVectors(t *testing.T) {
	raw, err := os.ReadFile("testdata/vetores_firmware.json")
	if err != nil {
		t.Skip("vetores do firmware ausentes")
	}
	var v firmwareVectors
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	dev, _ := DevIDFromHex(v.DevID)
	kAuth, kRec := mustHex(t, v.KAuth), mustHex(t, v.KRec)
	for _, h := range v.Handshake {
		p, tag := mustHex(t, h.Payload), mustHex(t, h.Tag)
		e := TagInput(Type(h.Type), dev, p)
		if !CompareTag(macSHA(kAuth, e)[:16], tag) {
			t.Errorf("%s: tag de handshake diverge do firmware", h.Name)
		}
		switch Type(h.Type) {
		case ChallengeType:
			if _, _, err := DecodeChallenge(append(append([]byte{}, p...), tag...)); err != nil {
				t.Errorf("%s: %v", h.Name, err)
			}
		case ResponseType:
			if _, _, _, err := DecodeResponse(append(append([]byte{}, p...), tag...)); err != nil {
				t.Errorf("%s: %v", h.Name, err)
			}
		case AckType:
			if _, _, err := DecodeAck(append(append([]byte{}, p...), tag...)); err != nil {
				t.Errorf("%s: %v", h.Name, err)
			}
		}
	}
	for _, b := range v.Blocks {
		d, tag := mustHex(t, b.Data), mustHex(t, b.Tag)
		if !CompareTag(macSHA(kRec, TagInput(BlockType, dev, d)), tag) {
			t.Errorf("%s: tag de bloco diverge do firmware", b.Name)
		}
		bl, err := DecodeBlock(append(append([]byte{}, d...), tag...), ProfileHMAC)
		if err != nil {
			t.Fatalf("%s: %v", b.Name, err)
		}
		c, err := ReadRecords(bl.Data)
		if err != nil || c.Marker == nil {
			t.Fatalf("%s: marcador esperado (%v)", b.Name, err)
		}
	}
}

func TestBuilderAndDecoder(t *testing.T) {
	m := NewBlockBuilder(9, 2, 1000)
	m.PID(12, 0, 23, 112)
	m.GPS(65535, -1, 2, 3)
	m.PID(13, 40, 62, 0)
	d := m.Data()
	c, err := ReadRecords(d)
	if err != nil || len(c.PIDs) != 2 || len(c.GPS) != 1 || c.DtMax != 65535 || c.DtMin != 0 {
		t.Fatalf("%+v %v", c, err)
	}
	if c.GPS[0].LatE7 != -1 || c.PIDs[0].B != 112 {
		t.Fatal("campos")
	}
	tlv := BuildBlockTLV(d, make([]byte, 32))
	l, _, _ := ReadTLV(tlv)
	if _, err := DecodeBlock(l.V, ProfileHMAC); err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeBlock(l.V, ProfileNone); err == nil {
		t.Fatal("perfil none deveria divergir no tamanho")
	}
}

func TestMarkerWithOthers(t *testing.T) {
	m := NewBlockBuilder(1, 1, 0)
	m.PID(4, 0, 1, 0)
	m.buf = append(m.buf, BuildMarker(1, 1, 0, RegBoot, 1, 1)[BlockHeaderSize:]...)
	if _, err := ReadRecords(m.Data()); err == nil {
		t.Fatal("esperava erro")
	}
	mk := BuildMarker(5, 1, 0, RegLoss, 2, 4)
	c, err := ReadRecords(mk)
	if err != nil || c.Marker == nil || c.Marker.FromSeq != 2 || c.Marker.ToSeq != 4 || len(mk) != 23 {
		t.Fatalf("%+v %v", c, err)
	}
}

func TestLeftoverBytes(t *testing.T) {
	m := NewBlockBuilder(1, 1, 0)
	m.PID(4, 0, 1, 0)
	d := append(m.Data(), 0x04, 0x00) // 2 bytes sobrando (registro truncado)
	if _, err := ReadRecords(d); err == nil {
		t.Fatal("esperava erro")
	}
}

func TestInvalidSizes(t *testing.T) {
	for _, n := range []int{44, 46, 48, 49, 51} {
		if _, _, err := DecodeChallenge(make([]byte, n)); err == nil {
			t.Errorf("desafio %d", n)
		}
	}
	if _, _, _, err := DecodeResponse(make([]byte, 49)); err == nil {
		t.Error("resposta")
	}
	if _, _, err := DecodeAck(make([]byte, 24)); err == nil {
		t.Error("ack")
	}
	v := make([]byte, 45)
	v[28] = 0x02
	if _, _, err := DecodeChallenge(v); err != ErrReservedFlags {
		t.Errorf("flags reservadas: %v", err)
	}
	v = make([]byte, 45)
	v[28] = 1 // diag exige 47
	if _, _, err := DecodeChallenge(v); err == nil {
		t.Error("diag com 45")
	}
}

func TestSize1024(t *testing.T) {
	m := NewBlockBuilder(1, 1, 0)
	for m.PID(4, 0, 1, 0) {
	}
	d := m.Data()
	if len(d) > 1024 {
		t.Fatal(len(d))
	}
	// força exatamente 1024: 12 + 1012 não é múltiplo de 5; usar GPS (15) não ajuda; checa o limite aceito
	v := append(append([]byte{}, d...), make([]byte, 32)...)
	if _, err := DecodeBlock(v, ProfileHMAC); err != nil {
		t.Fatal(err)
	}
	g := make([]byte, 1025+32)
	g[10], g[11] = 0x01, 0x04 // tamanho = 1025
	if _, err := DecodeBlock(g, ProfileHMAC); err == nil {
		t.Fatal("1025 deveria falhar")
	}
	p := make([]byte, 16+32)
	p[10] = 16
	if _, err := DecodeBlock(p, ProfileHMAC); err == nil {
		t.Fatal("16 deveria falhar")
	}
}

func TestBatch(t *testing.T) {
	var body []byte
	for i := 0; i < 3; i++ {
		body = AppendTLV(body, BlockType, make([]byte, 49))
	}
	l, err := ReadBatch(body, 3)
	if err != nil || len(l) != 3 {
		t.Fatal(err)
	}
	if _, err := ReadBatch(body, 2); err != ErrBatchTooLarge {
		t.Fatal(err)
	}
	if _, err := ReadBatch(nil, 2); err != ErrEmptyBatch {
		t.Fatal(err)
	}
	if _, err := ReadBatch(body[:len(body)-1], 5); err != ErrTLVLength {
		t.Fatal(err)
	}
	if _, err := ReadBatch([]byte{3, 1}, 5); err != ErrTLVShort {
		t.Fatal(err)
	}
}

func TestDevID(t *testing.T) {
	if _, err := DevIDFromHex("zz"); err == nil {
		t.Fatal()
	}
	if _, err := DevIDFromHex("544343454C544101"); err != nil {
		t.Fatal(err)
	}
	d, _ := DevIDFromHex("544343454C544101")
	if d.Hex() != "544343454c544101" {
		t.Fatal(d.Hex())
	}
}

func FuzzReadBatch(f *testing.F) {
	f.Add(AppendTLV(nil, BlockType, make([]byte, 49)))
	f.Fuzz(func(t *testing.T, b []byte) { _, _ = ReadBatch(b, 32) })
}

func FuzzReadRecords(f *testing.F) {
	f.Add(BuildMarker(1, 1, 0, RegBoot, 1, 2))
	f.Fuzz(func(t *testing.T, b []byte) { _, _ = ReadRecords(b) })
}

func BenchmarkReadRecords(b *testing.B) {
	m := NewBlockBuilder(1, 1, 0)
	for m.PID(12, 1, 2, 3) {
	}
	d := m.Data()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = ReadRecords(d)
	}
}

func TestMaxBlockTagSizeCoversEveryProfile(t *testing.T) {
	for _, p := range []Profile{ProfileHMAC, ProfileAsym, ProfileNone} {
		if p.BlockTagSize > MaxBlockTagSize {
			t.Fatalf("perfil %s: tag de %d B acima de MaxBlockTagSize (%d)", p.Name, p.BlockTagSize, MaxBlockTagSize)
		}
	}
	if MaxBlockTLVSize != 1091 {
		t.Fatalf("MaxBlockTLVSize = %d", MaxBlockTLVSize)
	}
}
