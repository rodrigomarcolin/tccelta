package handshake

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"tccelta/backend/internal/casodeuso/porta"
	"tccelta/backend/internal/casodeuso/porta/portateste"
	"tccelta/backend/internal/dominio/dongle"
	dhs "tccelta/backend/internal/dominio/handshake"
	"tccelta/backend/internal/dominio/ocorrencia"
	"tccelta/backend/internal/dominio/protocolo"
	"tccelta/backend/internal/repositorio/memoria"
)

var (
	dev   = protocolo.DevID{0, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77}
	kAuth = []byte("0123456789abcdef0123456789abcdef")
	kRec  = []byte("fedcba9876543210fedcba9876543210")
	t0    = time.Unix(1_800_000_000, 0)
)

type fakeToken struct{}

func (fakeToken) Emitir(_ context.Context, _ protocolo.DevID, cs uint32) (porta.TokenEmitido, error) {
	return porta.TokenEmitido{Token: "tk", JTI: uuid.New(), ExpiraEm: t0.Add(time.Hour)}, nil
}
func (fakeToken) Validade() time.Duration { return time.Hour }

type amb struct {
	bd        *memoria.Banco
	kms       *portateste.Memoria
	relogio   *portateste.RelogioAjustavel
	emitir    *EmitirDesafio
	responder *ResponderDesafio
	params    Parametros
}

func novo(t *testing.T) *amb {
	t.Helper()
	bd := memoria.Novo()
	bd.AdicionarDongle(dongle.Dongle{ID: 7, DevID: dev, Perfil: protocolo.PerfilHMAC, ChaveAuth: "ka", ChaveRec: "kr", Ativo: true})
	k := portateste.Nova()
	k.AdicionarDongle("ka", "kr", kAuth, kRec)
	rel := &portateste.RelogioAjustavel{T: t0}
	p := Parametros{DesafioValidade: 60 * time.Second, MaxPendentes: 2, AncoraRTTMax: 2 * time.Second, TLatSugerido: time.Minute}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	repos := bd.Repositorios()
	return &amb{bd: bd, kms: k, relogio: rel, params: p,
		emitir:    NovoEmitirDesafio(repos, k, portateste.Nop{}, rel.Agora, log, p),
		responder: NovoResponderDesafio(repos, bd, k, fakeToken{}, portateste.Nop{}, rel.Agora, log, p)}
}

func macTag(chave []byte, tipo protocolo.Tipo, payload []byte) []byte {
	m := hmac.New(sha256.New, chave)
	m.Write(protocolo.EntradaTag(tipo, dev, payload))
	return m.Sum(nil)[:protocolo.TamTagAuth]
}

// responder monta a resposta do dongle ao desafio emitido.
func (a *amb) resposta(t *testing.T, d DesafioEmitido, tail, head uint32, ruim bool) []byte {
	t.Helper()
	tl, _, err := protocolo.LerTLV(d.TLV)
	if err != nil {
		t.Fatal(err)
	}
	des, _, err := protocolo.DecodificarDesafio(tl.V)
	if err != nil {
		t.Fatal(err)
	}
	r := protocolo.Resposta{CS: des.CS, NS: des.NS, TailSeq: tail, HeadSeq: head, BootID: 1, TMs: 5000}
	tag := macTag(kAuth, protocolo.TipoResposta, r.Payload())
	if ruim {
		tag[0] ^= 1
	}
	return r.Montar(tag)
}

func (a *amb) pedir(t *testing.T) DesafioEmitido {
	t.Helper()
	d, err := a.emitir.Executar(context.Background(), EmitirDesafioEntrada{DevID: dev})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func (a *amb) responderCom(tlv []byte) (ResultadoHandshake, error) {
	return a.responder.Executar(context.Background(), ResponderDesafioEntrada{DevID: dev, S2: a.relogio.Agora(), TLV: tlv})
}

func TestFluxoFeliz(t *testing.T) {
	a := novo(t)
	d := a.pedir(t)
	// o desafio é assinado com K_auth
	tl, _, _ := protocolo.LerTLV(d.TLV)
	des, tag, _ := protocolo.DecodificarDesafio(tl.V)
	if !protocolo.CompararTag(macTag(kAuth, protocolo.TipoDesafio, des.Payload()), tag) {
		t.Fatal("tag do desafio não confere")
	}
	a.relogio.Avancar(300 * time.Millisecond)
	res, err := a.responderCom(a.resposta(t, d, 1, 0, false))
	if err != nil {
		t.Fatal(err)
	}
	if res.Token != "tk" || !res.ComAncora || res.AckSeq != 0 || res.ValidadeS != 3600 || res.TLatSugerido != time.Minute {
		t.Fatalf("%+v", res)
	}
	est := a.bd.Estado(7)
	if est.CConf != int64(des.CS) || len(est.UltimoAck) == 0 {
		t.Fatalf("%+v", est)
	}
	g, _ := a.bd.Desafio(7, int64(des.CS))
	if g.Resultado != dhs.ResultadoAceito || len(a.bd.Ancoras) != 1 || len(a.bd.Notificacoes) != 1 {
		t.Fatalf("%+v ancoras=%d", g, len(a.bd.Ancoras))
	}
}

func TestDesafiosPendentesComRetryAfter(t *testing.T) {
	a := novo(t)
	a.pedir(t)
	a.pedir(t)
	d, err := a.emitir.Executar(context.Background(), EmitirDesafioEntrada{DevID: dev})
	if !errors.Is(err, dhs.ErrDesafiosPendentes) || d.RetryAfter != 20*time.Second {
		t.Fatalf("%v %v", err, d.RetryAfter)
	}
}

func TestTagInvalidaMarcaODesafio(t *testing.T) {
	a := novo(t)
	d := a.pedir(t)
	_, err := a.responderCom(a.resposta(t, d, 1, 0, true))
	if !errors.Is(err, dhs.ErrTagInvalida) {
		t.Fatal(err)
	}
	if g, _ := a.bd.Desafio(7, 1); g.Resultado != "tag_invalida" {
		t.Fatalf("%+v", g)
	}
	if a.bd.Estado(7).CConf != 0 {
		t.Fatal("estado não pode mudar")
	}
}

func TestCSAntigo(t *testing.T) {
	a := novo(t)
	a.bd.DefinirEstado(7, dongle.Estado{CConf: 100})
	d := a.pedir(t) // c_S = 1 <= c_conf
	if _, err := a.responderCom(a.resposta(t, d, 1, 0, false)); !errors.Is(err, dhs.ErrCSAntigo) {
		t.Fatal(err)
	}
}

func TestDesafioUsadoUmaVez(t *testing.T) {
	a := novo(t)
	d := a.pedir(t)
	r := a.resposta(t, d, 1, 0, false)
	if _, err := a.responderCom(r); err != nil {
		t.Fatal(err)
	}
	if _, err := a.responderCom(r); !errors.Is(err, dhs.ErrDesafioInvalido) {
		t.Fatal(err)
	}
}

func TestDesafioVencido(t *testing.T) {
	a := novo(t)
	d := a.pedir(t)
	a.relogio.Avancar(2 * time.Minute)
	if _, err := a.responderCom(a.resposta(t, d, 1, 0, false)); !errors.Is(err, dhs.ErrDesafioInvalido) {
		t.Fatal(err)
	}
}

func TestTailInconsistenteFinalizaSemMudarEstado(t *testing.T) {
	a := novo(t)
	d := a.pedir(t)
	_, err := a.responderCom(a.resposta(t, d, 10, 3, false)) // tail > head+1
	if !errors.Is(err, dhs.ErrRespostaInconsist) {
		t.Fatal(err)
	}
	g, _ := a.bd.Desafio(7, 1)
	if g.Resultado != dhs.ResultadoInconsistente || a.bd.Estado(7).CConf != 0 {
		t.Fatalf("%+v %+v", g, a.bd.Estado(7))
	}
}

func TestOverflowRegistraLacuna(t *testing.T) {
	a := novo(t)
	a.bd.DefinirEstado(7, dongle.Estado{SeqContiguo: 5, HeadSeq: 5})
	d := a.pedir(t)
	res, err := a.responderCom(a.resposta(t, d, 20, 30, false))
	if err != nil {
		t.Fatal(err)
	}
	if res.AckSeq != 19 || len(a.bd.Lacunas) != 1 {
		t.Fatalf("%+v %+v", res, a.bd.Lacunas)
	}
	l := a.bd.Lacunas[0]
	if l.Causa != ocorrencia.CausaOverflow || l.From != 6 || l.To != 19 || l.CS == nil || *l.CS != 1 {
		t.Fatalf("%+v", l)
	}
}

func TestAncoraRTTAlto(t *testing.T) {
	a := novo(t)
	d := a.pedir(t)
	a.relogio.Avancar(5 * time.Second) // acima de AncoraRTTMax
	res, err := a.responderCom(a.resposta(t, d, 1, 0, false))
	if err != nil {
		t.Fatal(err)
	}
	if res.ComAncora || len(a.bd.Ancoras) != 0 {
		t.Fatalf("%+v", res)
	}
	tl, _, _ := protocolo.LerTLV(res.AckTLV)
	ack, _, _ := protocolo.DecodificarAck(tl.V)
	if ack.Flags&protocolo.FlagAckAncoraFalhou == 0 {
		t.Fatal("ACK deveria sinalizar âncora falhou")
	}
	if g, _ := a.bd.Desafio(7, 1); g.Resultado != dhs.ResultadoAceitoSemAncora {
		t.Fatalf("%+v", g)
	}
}

func TestAcesso(t *testing.T) {
	a := novo(t)
	if _, err := a.emitir.Executar(context.Background(), EmitirDesafioEntrada{DevID: protocolo.DevID{9}}); !errors.Is(err, dongle.ErrDesconhecido) {
		t.Fatal(err)
	}
	a.bd.AdicionarDongle(dongle.Dongle{ID: 8, DevID: protocolo.DevID{8}, Perfil: protocolo.PerfilNone, Ativo: true})
	if _, err := a.emitir.Executar(context.Background(), EmitirDesafioEntrada{DevID: protocolo.DevID{8}}); !errors.Is(err, dongle.ErrPerfilNaoPermitido) {
		t.Fatal(err)
	}
	a.bd.AdicionarDongle(dongle.Dongle{ID: 9, DevID: protocolo.DevID{9}, Perfil: protocolo.PerfilHMAC, Ativo: false})
	if _, err := a.emitir.Executar(context.Background(), EmitirDesafioEntrada{DevID: protocolo.DevID{9}}); !errors.Is(err, dongle.ErrInativo) {
		t.Fatal(err)
	}
}

func TestKMSIndisponivel(t *testing.T) {
	a := novo(t)
	var f error = porta.ErrKMSIndisponivel
	a.kms.Falha.Store(&f)
	if _, err := a.emitir.Executar(context.Background(), EmitirDesafioEntrada{DevID: dev}); !errors.Is(err, porta.ErrKMSIndisponivel) {
		t.Fatal(err)
	}
}
