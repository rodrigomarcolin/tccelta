package ingestao

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

	"tccelta/backend/internal/casodeuso/porta"
	"tccelta/backend/internal/casodeuso/porta/portateste"
	"tccelta/backend/internal/dominio/dongle"
	ding "tccelta/backend/internal/dominio/ingestao"
	"tccelta/backend/internal/dominio/ocorrencia"
	"tccelta/backend/internal/dominio/pid"
	"tccelta/backend/internal/dominio/protocolo"
	"tccelta/backend/internal/repositorio/memoria"
)

var (
	dev   = protocolo.DevID{0, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77}
	kAuth = []byte("0123456789abcdef0123456789abcdef")
	kRec  = []byte("fedcba9876543210fedcba9876543210")
	t0    = time.Unix(1_800_000_000, 0)
)

func f(v float64) *float64 { return &v }

type amb struct {
	bd      *memoria.Banco
	kms     *portateste.Memoria
	receber *ReceberLote
	conf    *ObterConfirmacao
	sessao  porta.Sessao
}

func novo(t *testing.T) *amb {
	t.Helper()
	bd := memoria.Novo()
	bd.AdicionarDongle(dongle.Dongle{ID: 7, DevID: dev, Perfil: protocolo.PerfilHMAC, ChaveAuth: "ka", ChaveRec: "kr", Ativo: true})
	k := portateste.Nova()
	k.AdicionarDongle("ka", "kr", kAuth, kRec)
	cat := pid.DeMapa([]pid.Def{
		{PID: 4, NBytes: 1, Fator: 100.0 / 255, Min: f(0), Max: f(100)},
		{PID: 12, NBytes: 2, Fator: 0.25, Min: f(0), Max: f(16383.75)},
	})
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	p := Parametros{LoteMax: 32, VerifyLote: 2, VerifyParalelo: 2}
	repos := bd.Repositorios()
	return &amb{bd: bd, kms: k,
		receber: NovoReceberLote(repos, bd, k, cat, portateste.Nop{}, portateste.RelogioFixo(t0), log, p),
		conf:    NovoObterConfirmacao(repos, false),
		sessao:  porta.Sessao{Dev: dev, CS: 3, JTI: uuid.New(), ExpiraEm: t0.Add(time.Hour)}}
}

func mac(chave []byte, tipo protocolo.Tipo, payload []byte) []byte {
	m := hmac.New(sha256.New, chave)
	m.Write(protocolo.EntradaTag(tipo, dev, payload))
	return m.Sum(nil)
}

// bloco monta o TLV de um bloco de dados (PID 12) com tag K_rec; ruim corrompe a tag.
type bloco struct {
	seq  uint32
	boot uint16
	pid  uint8 // 0 => 12
	ruim bool
	raw  []byte // dados prontos (marcadores)
}

func (b bloco) tlv() []byte {
	dados := b.raw
	if dados == nil {
		m := protocolo.NovoMontadorBloco(b.seq, b.boot, 1000)
		p := b.pid
		if p == 0 {
			p = 12
		}
		m.PID(p, 0, 0x17, 0x70)
		m.GPS(10, -235505200, -466333100, 760000)
		dados = m.Dados()
	}
	tag := mac(kRec, protocolo.TipoBloco, dados)
	if b.ruim {
		tag[0] ^= 1
	}
	return protocolo.MontarBlocoTLV(dados, tag)
}

func lote(bs ...bloco) []byte {
	var out []byte
	for _, b := range bs {
		out = append(out, b.tlv()...)
	}
	return out
}

func seqs(a, b uint32) []bloco {
	var o []bloco
	for s := a; s <= b; s++ {
		o = append(o, bloco{seq: s, boot: 1})
	}
	return o
}

func (a *amb) enviar(t *testing.T, corpo []byte) (ResultadoLote, error) {
	t.Helper()
	return a.receber.Executar(context.Background(), ReceberLoteEntrada{Sessao: a.sessao, Corpo: corpo})
}

func ackDe(t *testing.T, tlv []byte) protocolo.Ack {
	t.Helper()
	a, ok := ackDecodificado(tlv)
	if !ok {
		t.Fatal("ACK inválido")
	}
	return a
}

func TestLoteCompleto(t *testing.T) {
	a := novo(t)
	r, err := a.enviar(t, lote(seqs(1, 3)...))
	if err != nil {
		t.Fatal(err)
	}
	if r.Aceitos != 3 || r.AckSeq != 3 || r.Interrompido || r.Acao != ding.AcaoNenhuma {
		t.Fatalf("%+v", r)
	}
	if ack := ackDe(t, r.AckTLV); ack.AckSeq != 3 || ack.CConf != 0 {
		t.Fatalf("%+v", ack)
	}
	tl, _, _ := protocolo.LerTLV(r.AckTLV)
	_, tag, _ := protocolo.DecodificarAck(tl.V)
	ack := ackDe(t, r.AckTLV)
	if !protocolo.CompararTag(mac(kAuth, protocolo.TipoAck, ack.Payload())[:protocolo.TamTagAuth], tag) {
		t.Fatal("tag do ACK não confere")
	}
	if len(a.bd.Brutos) != 3 || len(a.bd.Leituras) != 3 || len(a.bd.Posicoes) != 3 {
		t.Fatalf("brutos=%d leituras=%d posicoes=%d", len(a.bd.Brutos), len(a.bd.Leituras), len(a.bd.Posicoes))
	}
	if est := a.bd.Estado(7); est.SeqContiguo != 3 || !bytes.Equal(est.UltimoAck, r.AckTLV) {
		t.Fatalf("%+v", est)
	}
	// ACK provável acertado: uma única chamada de HMAC (pré-assinatura reaproveitada na transação)
	if n := a.kms.ChamadasHMAC.Load(); n != 1 {
		t.Fatalf("HMAC chamado %d vezes, esperado 1", n)
	}
	// o verify foi em lotes de 2: 3 blocos => 2 chamadas
	if n := a.kms.ChamadasVerify.Load(); n != 2 {
		t.Fatalf("verify chamado %d vezes, esperado 2", n)
	}
}

func TestReenvioDuplicadoReaproveitaAck(t *testing.T) {
	a := novo(t)
	corpo := lote(seqs(1, 3)...)
	r1, _ := a.enviar(t, corpo)
	hmacs, verifs := a.kms.ChamadasHMAC.Load(), a.kms.ChamadasVerify.Load()
	r2, err := a.enviar(t, corpo)
	if err != nil {
		t.Fatal(err)
	}
	if r2.Aceitos != 0 || r2.Duplicados != 3 || !bytes.Equal(r2.AckTLV, r1.AckTLV) {
		t.Fatalf("%+v", r2)
	}
	if a.kms.ChamadasHMAC.Load() != hmacs || a.kms.ChamadasVerify.Load() != verifs {
		t.Fatal("duplicados não devem chamar o KMS")
	}
	if len(a.bd.Brutos) != 3 {
		t.Fatal("nada novo para gravar")
	}
}

func TestSaltoInterrompe(t *testing.T) {
	a := novo(t)
	r, err := a.enviar(t, lote(bloco{seq: 1, boot: 1}, bloco{seq: 2, boot: 1}, bloco{seq: 4, boot: 1}))
	if err != nil {
		t.Fatal(err)
	}
	if r.AckSeq != 2 || !r.Interrompido || r.Motivo != ding.MotivoSalto || r.Acao != ding.AcaoReler || r.SemProgresso {
		t.Fatalf("%+v", r)
	}
}

func TestTagInvalidaRejeitaEInterrompe(t *testing.T) {
	a := novo(t)
	r, err := a.enviar(t, lote(bloco{seq: 1, boot: 1}, bloco{seq: 2, boot: 1, ruim: true}, bloco{seq: 3, boot: 1}))
	if err != nil {
		t.Fatal(err)
	}
	if r.AckSeq != 1 || r.Motivo != ding.MotivoTagInvalida || r.SemProgresso || !r.Interrompido {
		t.Fatalf("%+v", r)
	}
	if len(a.bd.Rejeitados) != 1 || a.bd.Rejeitados[0].Motivo != ding.MotivoTagInvalida || a.bd.Rejeitados[0].Seq != 2 ||
		a.bd.Rejeitados[0].TokenJTI != a.sessao.JTI {
		t.Fatalf("%+v", a.bd.Rejeitados)
	}
	// a previsão do ACK (tudo válido) errou: HMAC extra dentro da transação
	if n := a.kms.ChamadasHMAC.Load(); n != 2 {
		t.Fatalf("HMAC chamado %d vezes, esperado 2 (pré + tx)", n)
	}
}

func TestPrimeiroBlocoComTagInvalidaSemProgresso(t *testing.T) {
	a := novo(t)
	r, err := a.enviar(t, lote(bloco{seq: 1, boot: 1, ruim: true}))
	if err != nil {
		t.Fatal(err)
	}
	if !r.SemProgresso || r.AckSeq != 0 {
		t.Fatalf("%+v", r)
	}
}

func TestPIDDesconhecido(t *testing.T) {
	a := novo(t)
	r, err := a.enviar(t, lote(bloco{seq: 1, boot: 1}, bloco{seq: 2, boot: 1, pid: 0x99}))
	if err != nil {
		t.Fatal(err)
	}
	if r.AckSeq != 1 || r.Motivo != ding.MotivoPIDDesconhecido || r.Acao != ding.AcaoAguardar {
		t.Fatalf("%+v", r)
	}
	if len(a.bd.Rejeitados) != 1 || len(a.bd.Alertas) != 1 || a.bd.Alertas[0].Tipo != ocorrencia.AlertaPIDDesconhecido {
		t.Fatalf("%+v %+v", a.bd.Rejeitados, a.bd.Alertas)
	}
}

func TestMarcadoresLossEBoot(t *testing.T) {
	a := novo(t)
	loss := bloco{raw: protocolo.MontarMarcador(3, 1, 0, protocolo.RegLoss, 2, 3)}
	r, err := a.enviar(t, lote(bloco{seq: 1, boot: 1}, loss))
	if err != nil {
		t.Fatal(err)
	}
	if r.AckSeq != 3 || len(a.bd.Lacunas) != 1 || a.bd.Lacunas[0].Causa != ocorrencia.CausaLoss ||
		a.bd.Lacunas[0].From != 2 || a.bd.Lacunas[0].To != 3 {
		t.Fatalf("%+v %+v", r, a.bd.Lacunas)
	}
	boot := bloco{raw: protocolo.MontarMarcador(8, 2, 0, protocolo.RegBoot, 4, 7)}
	r, err = a.enviar(t, lote(boot, bloco{seq: 9, boot: 2}))
	if err != nil {
		t.Fatal(err)
	}
	if r.AckSeq != 9 || len(a.bd.Lacunas) != 2 || a.bd.Lacunas[1].Causa != ocorrencia.CausaBoot ||
		a.bd.Lacunas[1].From != 4 || a.bd.Lacunas[1].To != 7 {
		t.Fatalf("%+v %+v", r, a.bd.Lacunas)
	}
}

func TestGeracaoAvanca(t *testing.T) {
	a := novo(t)
	a.bd.DefinirEstado(7, dongle.Estado{SeqContiguo: 4, MaiorSeqVisto: 4, UltimoBootID: 65535})
	if _, err := a.enviar(t, lote(bloco{seq: 5, boot: 1})); err != nil { // boot_id voltou: nova geração
		t.Fatal(err)
	}
	if est := a.bd.Estado(7); est.GeracaoBoot != 1 || est.UltimoBootID != 1 {
		t.Fatalf("%+v", est)
	}
	if len(a.bd.Alertas) != 1 || a.bd.Alertas[0].Tipo != ocorrencia.AlertaGeracaoAvancou {
		t.Fatalf("%+v", a.bd.Alertas)
	}
}

func TestLoteMalformadoEGrande(t *testing.T) {
	a := novo(t)
	if _, err := a.enviar(t, []byte{0xFF, 0x00}); !errors.Is(err, ding.ErrLoteMalformado) {
		t.Fatal(err)
	}
	var bs []bloco
	for s := uint32(1); s <= 40; s++ {
		bs = append(bs, bloco{seq: s, boot: 1})
	}
	if _, err := a.enviar(t, lote(bs...)); !errors.Is(err, ding.ErrLoteGrande) {
		t.Fatal(err)
	}
}

func TestKMSIndisponivelNaoGravaNada(t *testing.T) {
	a := novo(t)
	var falha error = porta.ErrKMSIndisponivel
	a.kms.Falha.Store(&falha)
	if _, err := a.enviar(t, lote(seqs(1, 2)...)); !errors.Is(err, porta.ErrKMSIndisponivel) {
		t.Fatal(err)
	}
	if len(a.bd.Brutos) != 0 || a.bd.Estado(7).SeqContiguo != 0 {
		t.Fatal("nada pode ser gravado")
	}
}

func TestObterConfirmacao(t *testing.T) {
	a := novo(t)
	if _, err := a.conf.Executar(context.Background(), ObterConfirmacaoEntrada{DevID: dev}); !errors.Is(err, ding.ErrSemConfirmacao) {
		t.Fatal(err)
	}
	r, _ := a.enviar(t, lote(seqs(1, 1)...))
	ack, err := a.conf.Executar(context.Background(), ObterConfirmacaoEntrada{DevID: dev})
	if err != nil || !bytes.Equal(ack, r.AckTLV) {
		t.Fatalf("%v %x", err, ack)
	}
	n := a.kms.ChamadasHMAC.Load()
	_, _ = a.conf.Executar(context.Background(), ObterConfirmacaoEntrada{DevID: dev})
	if a.kms.ChamadasHMAC.Load() != n {
		t.Fatal("a confirmação não chama o KMS")
	}
}

func TestAcessoNegado(t *testing.T) {
	a := novo(t)
	a.bd.AdicionarDongle(dongle.Dongle{ID: 8, DevID: protocolo.DevID{8}, Perfil: protocolo.PerfilNone, Ativo: true})
	a.sessao.Dev = protocolo.DevID{8}
	if _, err := a.enviar(t, lote(seqs(1, 1)...)); !errors.Is(err, dongle.ErrPerfilNaoPermitido) {
		t.Fatal(err)
	}
	a.bd.AdicionarDongle(dongle.Dongle{ID: 9, DevID: protocolo.DevID{9}, Perfil: protocolo.PerfilHMAC, Ativo: false})
	a.sessao.Dev = protocolo.DevID{9}
	if _, err := a.enviar(t, lote(seqs(1, 1)...)); !errors.Is(err, dongle.ErrInativo) {
		t.Fatal(err)
	}
}
