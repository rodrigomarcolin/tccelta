package protocolo

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

type vetores struct {
	DevID string `json:"dev_id"`
	KAuth string `json:"k_auth"`
	KRec  string `json:"k_rec"`
	Casos []struct {
		Nome        string          `json:"nome"`
		Tipo        int             `json:"tipo"`
		Campos      json.RawMessage `json:"campos"`
		EntradaHMAC string          `json:"entrada_hmac"`
		HMACCompl   string          `json:"hmac_completo"`
		Dados       string          `json:"dados"`
		Tag         string          `json:"tag"`
		TLV         string          `json:"tlv"`
	} `json:"casos"`
}

func TestVetoresPython(t *testing.T) {
	raw, err := os.ReadFile("testdata/vetores.json")
	if err != nil {
		t.Fatal(err)
	}
	var v vetores
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	dev, err := DevIDDeHex(v.DevID)
	if err != nil {
		t.Fatal(err)
	}
	kAuth, kRec := mustHex(t, v.KAuth), mustHex(t, v.KRec)
	for _, c := range v.Casos {
		t.Run(c.Nome, func(t *testing.T) {
			tl, n, err := LerTLV(mustHex(t, c.TLV))
			if err != nil || n != len(mustHex(t, c.TLV)) {
				t.Fatalf("LerTLV: %v", err)
			}
			if int(tl.T) != c.Tipo {
				t.Fatalf("tipo %d", tl.T)
			}
			switch tl.T {
			case TipoDesafio:
				d, tag, err := DecodificarDesafio(tl.V)
				if err != nil {
					t.Fatal(err)
				}
				e := EntradaTag(TipoDesafio, dev, d.Payload())
				if hex.EncodeToString(e) != c.EntradaHMAC {
					t.Fatalf("entrada\n%x\n%s", e, c.EntradaHMAC)
				}
				if !CompararTag(macSHA(kAuth, e)[:16], tag) {
					t.Fatal("tag")
				}
				re, err := d.Montar(tag)
				if err != nil || !bytes.Equal(re, mustHex(t, c.TLV)) {
					t.Fatalf("remontar: %v", err)
				}
			case TipoResposta:
				r, p, tag, err := DecodificarResposta(tl.V)
				if err != nil {
					t.Fatal(err)
				}
				e := EntradaTag(TipoResposta, dev, p)
				if hex.EncodeToString(e) != c.EntradaHMAC || !CompararTag(macSHA(kAuth, e)[:16], tag) {
					t.Fatal("entrada/tag")
				}
				if !bytes.Equal(r.Montar(tag), mustHex(t, c.TLV)) {
					t.Fatal("remontar")
				}
			case TipoAck:
				a, tag, err := DecodificarAck(tl.V)
				if err != nil {
					t.Fatal(err)
				}
				e := EntradaTag(TipoAck, dev, a.Payload())
				if hex.EncodeToString(e) != c.EntradaHMAC || !CompararTag(macSHA(kAuth, e)[:16], tag) {
					t.Fatal("entrada/tag")
				}
				if !bytes.Equal(a.Montar(tag), mustHex(t, c.TLV)) {
					t.Fatal("remontar")
				}
			case TipoBloco:
				b, err := DecodificarBloco(tl.V, PerfilHMAC)
				if err != nil {
					t.Fatal(err)
				}
				if hex.EncodeToString(b.Dados) != c.Dados || hex.EncodeToString(b.Tag) != c.Tag {
					t.Fatal("dados/tag")
				}
				e := EntradaTag(TipoBloco, dev, b.Dados)
				if hex.EncodeToString(e) != c.EntradaHMAC || !CompararTag(macSHA(kRec, e), b.Tag) {
					t.Fatal("entrada/tag")
				}
				if _, err := LerRegistros(b.Dados); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

type vetoresFW struct {
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

func TestVetoresFirmware(t *testing.T) {
	raw, err := os.ReadFile("testdata/vetores_firmware.json")
	if err != nil {
		t.Skip("vetores do firmware ausentes")
	}
	var v vetoresFW
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	dev, _ := DevIDDeHex(v.DevID)
	kAuth, kRec := mustHex(t, v.KAuth), mustHex(t, v.KRec)
	for _, h := range v.Handshake {
		p, tag := mustHex(t, h.Payload), mustHex(t, h.Tag)
		e := EntradaTag(Tipo(h.Type), dev, p)
		if !CompararTag(macSHA(kAuth, e)[:16], tag) {
			t.Errorf("%s: tag de handshake diverge do firmware", h.Name)
		}
		switch Tipo(h.Type) {
		case TipoDesafio:
			if _, _, err := DecodificarDesafio(append(append([]byte{}, p...), tag...)); err != nil {
				t.Errorf("%s: %v", h.Name, err)
			}
		case TipoResposta:
			if _, _, _, err := DecodificarResposta(append(append([]byte{}, p...), tag...)); err != nil {
				t.Errorf("%s: %v", h.Name, err)
			}
		case TipoAck:
			if _, _, err := DecodificarAck(append(append([]byte{}, p...), tag...)); err != nil {
				t.Errorf("%s: %v", h.Name, err)
			}
		}
	}
	for _, b := range v.Blocks {
		d, tag := mustHex(t, b.Data), mustHex(t, b.Tag)
		if !CompararTag(macSHA(kRec, EntradaTag(TipoBloco, dev, d)), tag) {
			t.Errorf("%s: tag de bloco diverge do firmware", b.Name)
		}
		bl, err := DecodificarBloco(append(append([]byte{}, d...), tag...), PerfilHMAC)
		if err != nil {
			t.Fatalf("%s: %v", b.Name, err)
		}
		c, err := LerRegistros(bl.Dados)
		if err != nil || c.Marcador == nil {
			t.Fatalf("%s: marcador esperado (%v)", b.Name, err)
		}
	}
}

func TestMontadorEDecodificador(t *testing.T) {
	m := NovoMontadorBloco(9, 2, 1000)
	m.PID(12, 0, 23, 112)
	m.GPS(65535, -1, 2, 3)
	m.PID(13, 40, 62, 0)
	d := m.Dados()
	c, err := LerRegistros(d)
	if err != nil || len(c.PIDs) != 2 || len(c.GPS) != 1 || c.DtMax != 65535 || c.DtMin != 0 {
		t.Fatalf("%+v %v", c, err)
	}
	if c.GPS[0].LatE7 != -1 || c.PIDs[0].B != 112 {
		t.Fatal("campos")
	}
	tlv := MontarBlocoTLV(d, make([]byte, 32))
	l, _, _ := LerTLV(tlv)
	if _, err := DecodificarBloco(l.V, PerfilHMAC); err != nil {
		t.Fatal(err)
	}
	if _, err := DecodificarBloco(l.V, PerfilNone); err == nil {
		t.Fatal("perfil none deveria divergir no tamanho")
	}
}

func TestMarcadorComOutros(t *testing.T) {
	m := NovoMontadorBloco(1, 1, 0)
	m.PID(4, 0, 1, 0)
	m.buf = append(m.buf, MontarMarcador(1, 1, 0, RegBoot, 1, 1)[TamCabBloco:]...)
	if _, err := LerRegistros(m.Dados()); err == nil {
		t.Fatal("esperava erro")
	}
	mk := MontarMarcador(5, 1, 0, RegLoss, 2, 4)
	c, err := LerRegistros(mk)
	if err != nil || c.Marcador == nil || c.Marcador.FromSeq != 2 || c.Marcador.ToSeq != 4 || len(mk) != 23 {
		t.Fatalf("%+v %v", c, err)
	}
}

func TestSobraDeBytes(t *testing.T) {
	m := NovoMontadorBloco(1, 1, 0)
	m.PID(4, 0, 1, 0)
	d := append(m.Dados(), 0x04, 0x00) // 2 bytes sobrando (registro truncado)
	if _, err := LerRegistros(d); err == nil {
		t.Fatal("esperava erro")
	}
}

func TestTamanhosInvalidos(t *testing.T) {
	for _, n := range []int{44, 46, 48, 49, 51} {
		if _, _, err := DecodificarDesafio(make([]byte, n)); err == nil {
			t.Errorf("desafio %d", n)
		}
	}
	if _, _, _, err := DecodificarResposta(make([]byte, 49)); err == nil {
		t.Error("resposta")
	}
	if _, _, err := DecodificarAck(make([]byte, 24)); err == nil {
		t.Error("ack")
	}
	v := make([]byte, 45)
	v[28] = 0x02
	if _, _, err := DecodificarDesafio(v); err != ErrFlagsReservadas {
		t.Errorf("flags reservadas: %v", err)
	}
	v = make([]byte, 45)
	v[28] = 1 // diag exige 47
	if _, _, err := DecodificarDesafio(v); err == nil {
		t.Error("diag com 45")
	}
}

func TestTamanho1024(t *testing.T) {
	m := NovoMontadorBloco(1, 1, 0)
	for m.PID(4, 0, 1, 0) {
	}
	d := m.Dados()
	if len(d) > 1024 {
		t.Fatal(len(d))
	}
	// força exatamente 1024: 12 + 1012 não é múltiplo de 5; usar GPS (15) não ajuda; checa o limite aceito
	v := append(append([]byte{}, d...), make([]byte, 32)...)
	if _, err := DecodificarBloco(v, PerfilHMAC); err != nil {
		t.Fatal(err)
	}
	g := make([]byte, 1025+32)
	g[10], g[11] = 0x01, 0x04 // tamanho = 1025
	if _, err := DecodificarBloco(g, PerfilHMAC); err == nil {
		t.Fatal("1025 deveria falhar")
	}
	p := make([]byte, 16+32)
	p[10] = 16
	if _, err := DecodificarBloco(p, PerfilHMAC); err == nil {
		t.Fatal("16 deveria falhar")
	}
}

func TestLote(t *testing.T) {
	var corpo []byte
	for i := 0; i < 3; i++ {
		corpo = AnexarTLV(corpo, TipoBloco, make([]byte, 49))
	}
	l, err := LerLote(corpo, 3)
	if err != nil || len(l) != 3 {
		t.Fatal(err)
	}
	if _, err := LerLote(corpo, 2); err != ErrLoteGrande {
		t.Fatal(err)
	}
	if _, err := LerLote(nil, 2); err != ErrLoteVazio {
		t.Fatal(err)
	}
	if _, err := LerLote(corpo[:len(corpo)-1], 5); err != ErrTLVComprimento {
		t.Fatal(err)
	}
	if _, err := LerLote([]byte{3, 1}, 5); err != ErrTLVCurto {
		t.Fatal(err)
	}
}

func TestDevID(t *testing.T) {
	if _, err := DevIDDeHex("zz"); err == nil {
		t.Fatal()
	}
	if _, err := DevIDDeHex("544343454C544101"); err != nil {
		t.Fatal(err)
	}
	d, _ := DevIDDeHex("544343454C544101")
	if d.Hex() != "544343454c544101" {
		t.Fatal(d.Hex())
	}
}

func FuzzLerLote(f *testing.F) {
	f.Add(AnexarTLV(nil, TipoBloco, make([]byte, 49)))
	f.Fuzz(func(t *testing.T, b []byte) { _, _ = LerLote(b, 32) })
}

func FuzzLerRegistros(f *testing.F) {
	f.Add(MontarMarcador(1, 1, 0, RegBoot, 1, 2))
	f.Fuzz(func(t *testing.T, b []byte) { _, _ = LerRegistros(b) })
}

func BenchmarkLerRegistros(b *testing.B) {
	m := NovoMontadorBloco(1, 1, 0)
	for m.PID(12, 1, 2, 3) {
	}
	d := m.Dados()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = LerRegistros(d)
	}
}
