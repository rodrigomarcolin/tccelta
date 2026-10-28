// Package ingestao tem os casos de uso do lote de blocos (spec 05).
package ingestao

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"golang.org/x/sync/errgroup"

	"tccelta/backend/internal/casodeuso/porta"
	"tccelta/backend/internal/dominio/dongle"
	ding "tccelta/backend/internal/dominio/ingestao"
	"tccelta/backend/internal/dominio/ocorrencia"
	"tccelta/backend/internal/dominio/pid"
	"tccelta/backend/internal/dominio/protocolo"
	"tccelta/backend/internal/dominio/tempo"
	"tccelta/backend/internal/observabilidade"
	"tccelta/backend/internal/repositorio"
)

// Parametros são os ajustes da ingestão.
type Parametros struct {
	LoteMax            int
	VerifyLote         int
	VerifyParalelo     int
	PermitirPerfilNone bool
}

// ReceberLoteEntrada é o lote recebido: a sessão autenticada e o corpo bruto (TLVs de bloco).
type ReceberLoteEntrada struct {
	Sessao porta.Sessao
	Corpo  []byte
}

// ResultadoLote é a saída de ReceberLote.
type ResultadoLote struct {
	AckTLV       []byte
	AckSeq       uint32
	Interrompido bool
	Motivo       ding.Motivo
	Acao         ding.Acao
	SemProgresso bool
	Aceitos      int
	Duplicados   int
}

// ReceberLote implementa o algoritmo da spec 05 (POST /v1/dongles/blocos).
type ReceberLote struct {
	repos repositorio.Repositorios
	tx    repositorio.Transacionador
	kms   porta.Autenticador
	pids  pid.Consulta
	met   porta.MetricasIngestao
	agora porta.Relogio
	log   *slog.Logger
	p     Parametros
}

// NovoReceberLote liga o caso de uso às suas portas.
func NovoReceberLote(repos repositorio.Repositorios, tx repositorio.Transacionador, kms porta.Autenticador, pids pid.Consulta,
	met porta.MetricasIngestao, agora porta.Relogio, log *slog.Logger, p Parametros) *ReceberLote {
	return &ReceberLote{repos: repos, tx: tx, kms: kms, pids: pids, met: met, agora: agora, log: log, p: p}
}

func ackDecodificado(tlv []byte) (protocolo.Ack, bool) {
	t, n, err := protocolo.LerTLV(tlv)
	if err != nil || n != len(tlv) || t.T != protocolo.TipoAck {
		return protocolo.Ack{}, false
	}
	a, _, err := protocolo.DecodificarAck(t.V)
	return a, err == nil
}

// Executar processa o lote.
func (c *ReceberLote) Executar(ctx context.Context, e ReceberLoteEntrada) (ResultadoLote, error) {
	d, err := c.repos.Dongles.BuscarPorDevID(ctx, e.Sessao.Dev)
	if err != nil {
		return ResultadoLote{}, err
	}
	if err := d.VerificarAcesso(c.p.PermitirPerfilNone); err != nil {
		return ResultadoLote{}, err
	}

	// 1. forma
	tlvs, err := protocolo.LerLote(e.Corpo, c.p.LoteMax)
	if err != nil {
		if errors.Is(err, protocolo.ErrLoteGrande) {
			return ResultadoLote{}, ding.ErrLoteGrande
		}
		return ResultadoLote{}, fmt.Errorf("%w: %v", ding.ErrLoteMalformado, err)
	}
	blocos := make([]ding.BlocoIn, len(tlvs))
	for i, t := range tlvs {
		if t.T != protocolo.TipoBloco {
			return ResultadoLote{}, fmt.Errorf("%w: tipo 0x%02x", ding.ErrLoteMalformado, uint8(t.T))
		}
		b, err := protocolo.DecodificarBloco(t.V, d.Perfil)
		if err != nil {
			return ResultadoLote{}, fmt.Errorf("%w: %v", ding.ErrLoteMalformado, err)
		}
		blocos[i].B = b
	}

	// 2. snapshot
	snap, err := c.repos.Estados.LerSnapshot(ctx, d.ID)
	if err != nil {
		return ResultadoLote{}, err
	}
	var idx []int // blocos a verificar (seq > snapshot, sem repetir seq)
	visto := map[uint32]bool{}
	for i := range blocos {
		sq := blocos[i].Seq()
		if int64(sq) > snap.SeqContiguo && !visto[sq] {
			visto[sq] = true
			idx = append(idx, i)
		}
	}
	// repetições do mesmo seq dentro do lote herdam o resultado do primeiro
	for i := range blocos {
		if int64(blocos[i].Seq()) > snap.SeqContiguo {
			blocos[i].Verificado = true
		}
	}

	// 3. KMS em paralelo (verify do lote + pré-assinatura do ACK provável)
	validos := map[uint32]bool{}
	var tagPre []byte
	var ackPre protocolo.Ack
	preAssinado := false
	g, gctx := errgroup.WithContext(observabilidade.ComFaseKMS(ctx, observabilidade.FasePre))
	if len(idx) > 0 {
		lote := c.p.VerifyLote
		if lote <= 0 {
			lote = 32
		}
		par := c.p.VerifyParalelo
		if par <= 0 {
			par = 1
		}
		var sem = make(chan struct{}, par)
		var res = make([]bool, len(idx))
		for ini := 0; ini < len(idx); ini += lote {
			fim := min(ini+lote, len(idx))
			ini, fim := ini, fim
			g.Go(func() error {
				sem <- struct{}{}
				defer func() { <-sem }()
				ents := make([][]byte, 0, fim-ini)
				tags := make([][]byte, 0, fim-ini)
				for _, i := range idx[ini:fim] {
					ents = append(ents, protocolo.EntradaTag(protocolo.TipoBloco, d.DevID, blocos[i].B.Dados))
					tags = append(tags, blocos[i].B.Tag)
				}
				out, err := c.kms.VerificarBlocos(gctx, d.ChaveRec, ents, tags)
				if err != nil {
					return err
				}
				copy(res[ini:fim], out)
				return nil
			})
		}
		g.Go(func() error {
			// ACK provável: simula com todas as tags válidas sobre o snapshot
			sim := make([]ding.BlocoIn, len(blocos))
			copy(sim, blocos)
			for i := range sim {
				sim[i].Valido = true
			}
			r := ding.Aplicar(sim, ding.EstadoIn{SeqContiguo: uint32(snap.SeqContiguo)}, c.pids)
			prov := protocolo.Ack{AckSeq: r.NovoAck, CConf: uint32(snap.CConf)}
			if a, ok := ackDecodificado(snap.UltimoAck); ok && a == prov {
				return nil
			}
			macs, err := c.kms.HMACAuth(gctx, d.ChaveAuth, [][]byte{protocolo.EntradaTag(protocolo.TipoAck, d.DevID, prov.Payload())})
			if err != nil {
				return err
			}
			tagPre, ackPre, preAssinado = macs[0][:protocolo.TamTagAuth], prov, true
			return nil
		})
		if err := g.Wait(); err != nil {
			return ResultadoLote{}, err
		}
		for k, i := range idx {
			validos[blocos[i].Seq()] = res[k]
		}
	}
	for i := range blocos {
		if blocos[i].Verificado {
			blocos[i].Valido = validos[blocos[i].Seq()]
		}
	}

	// 4. transação
	var out ResultadoLote
	t0tx := time.Now()
	err = c.tx.EmTransacao(ctx, func(ctx context.Context, rp repositorio.Repositorios) error {
		out = ResultadoLote{}
		if err := rp.Dongles.Travar(ctx, d.ID); err != nil {
			return err
		}
		est, err := rp.Estados.Ler(ctx, d.ID)
		if err != nil {
			return err
		}
		agora := c.agora()
		r := ding.Aplicar(blocos, ding.EstadoIn{SeqContiguo: uint32(est.SeqContiguo), G: uint16(est.GeracaoBoot), U: uint16(est.UltimoBootID)}, c.pids)

		for _, rj := range r.Rejeitados {
			if err := c.rejeitar(ctx, rp, d, e.Sessao, rj, agora); err != nil {
				return err
			}
			if rj.Motivo == ding.MotivoPIDDesconhecido {
				det, _ := json.Marshal(map[string]any{"pid": rj.PID, "seq": rj.B.Cab.Seq})
				if err := rp.Ocorrencias.InserirAlerta(ctx, ocorrencia.Alerta{DongleID: d.ID, Tipo: ocorrencia.AlertaPIDDesconhecido, Detalhe: det}); err != nil {
					return err
				}
			}
		}

		if len(r.Aceitos) > 0 {
			if err := c.gravar(ctx, rp, d, e.Sessao, est, r, agora); err != nil {
				return err
			}
		}

		ackAtual, err := rp.Estados.LerUltimoAck(ctx, d.ID)
		if err != nil {
			return err
		}
		ack := protocolo.Ack{AckSeq: r.NovoAck, CConf: uint32(est.CConf)}
		var ackTLV []byte
		if a, ok := ackDecodificado(ackAtual); ok && a == ack {
			ackTLV = ackAtual
		} else if preAssinado && ackPre == ack {
			ackTLV = ack.Montar(tagPre)
		} else {
			macs, err := c.kms.HMACAuth(observabilidade.ComFaseKMS(ctx, observabilidade.FaseTx), d.ChaveAuth,
				[][]byte{protocolo.EntradaTag(protocolo.TipoAck, d.DevID, ack.Payload())})
			if err != nil {
				return err
			}
			ackTLV = ack.Montar(macs[0][:protocolo.TamTagAuth])
		}
		if int64(r.NovoAck) != est.SeqContiguo || int64(r.MaiorVisto) > est.MaiorSeqVisto || int32(r.G) != est.GeracaoBoot ||
			string(ackTLV) != string(ackAtual) {
			if err := rp.Estados.AtualizarIngestao(ctx, ding.AtualizacaoEstado{DongleID: d.ID, SeqContiguo: int64(r.NovoAck),
				MaiorVisto: int64(r.MaiorVisto), UltimoAck: ackTLV, Agora: agora, GeracaoBoot: int32(r.G), UltimoBootID: int32(r.U)}); err != nil {
				return err
			}
		}
		out = ResultadoLote{AckTLV: ackTLV, AckSeq: r.NovoAck, Motivo: r.Motivo, Acao: ding.AcaoDe(r.Motivo),
			Aceitos: len(r.Aceitos), Duplicados: r.Duplicados}
		ultimo := blocos[len(blocos)-1].Seq()
		out.Interrompido = r.NovoAck < ultimo
		out.SemProgresso = int64(r.NovoAck) == est.SeqContiguo && r.Motivo == ding.MotivoTagInvalida
		c.contar(r)
		return nil
	})
	c.met.DuracaoTxIngestao(time.Since(t0tx))
	if err != nil {
		return ResultadoLote{}, err
	}
	st := "completo"
	if out.SemProgresso {
		st = "sem_progresso"
	} else if out.Interrompido {
		st = "interrompido"
	}
	c.met.Lote(st)
	if out.Interrompido {
		c.log.Warn("lote.interrompido", "dongle", d.DevID.Hex(), "motivo", string(out.Motivo), "ack_seq", out.AckSeq)
	}
	return out, nil
}

func (c *ReceberLote) contar(r ding.ResultadoAplicacao) {
	c.met.Blocos("aceito", len(r.Aceitos))
	c.met.Blocos("duplicado", r.Duplicados)
	for _, rj := range r.Rejeitados {
		c.met.Blocos(string(rj.Motivo), 1)
	}
	if r.Motivo == ding.MotivoSalto {
		c.met.Blocos("descartado_apos_interrupcao", 1)
	}
}

func (c *ReceberLote) rejeitar(ctx context.Context, rp repositorio.Repositorios, d dongle.Dongle, sessao porta.Sessao,
	rj ding.Rejeicao, agora time.Time) error {
	h := sha256.Sum256(append(append([]byte{}, rj.B.Dados...), rj.B.Tag...))
	return rp.Blocos.RegistrarRejeitado(ctx, ding.Rejeitado{DongleID: d.ID, Seq: int64(rj.B.Cab.Seq), BootID: int32(rj.B.Cab.BootID),
		T0Ms: int64(rj.B.Cab.T0Ms), Payload: rj.B.Dados, Assinatura: rj.B.Tag, Motivo: rj.Motivo, Detalhe: rj.Detalhe,
		HashConteudo: h[:], TokenJTI: sessao.JTI, RecebidoEm: agora})
}

type chaveBoot struct{ g, b int32 }

type aggBoot struct {
	seqIni, seqFim, tIni, tFim int64
	desl                       float64
}

// gravar persiste os blocos aceitos e tudo que deriva deles.
func (c *ReceberLote) gravar(ctx context.Context, rp repositorio.Repositorios, d dongle.Dongle, sessao porta.Sessao,
	est dongle.Estado, r ding.ResultadoAplicacao, agora time.Time) error {
	// referências de t_ms por boot (gerações G e G+1)
	var ids []int32
	vistoID := map[int32]bool{}
	for _, a := range r.Aceitos {
		if a.Marcador == "" && !vistoID[int32(a.B.Cab.BootID)] {
			vistoID[int32(a.B.Cab.BootID)] = true
			ids = append(ids, int32(a.B.Cab.BootID))
		}
	}
	ref := map[chaveBoot]int64{}
	if len(ids) > 0 {
		rows, err := rp.Boots.TMsUltimo(ctx, d.ID, ids, est.GeracaoBoot, est.GeracaoBoot+1)
		if err != nil {
			return err
		}
		for _, x := range rows {
			ref[chaveBoot{x.Geracao, x.BootID}] = x.TMsUltimo
		}
	}
	brutos := make([]ding.BlocoBruto, len(r.Aceitos))
	var leituras []pid.Leitura
	var posicoes []pid.Posicao
	agg := map[chaveBoot]*aggBoot{}
	recMs := float64(agora.UnixMicro()) / 1000
	for i, a := range r.Aceitos {
		cab := a.B.Cab
		t0 := int64(cab.T0Ms)
		if a.Marcador == "" {
			k := chaveBoot{int32(a.Geracao), int32(cab.BootID)}
			rf, ok := ref[k]
			if !ok {
				rf = -1
			}
			t0 = tempo.Desembrulhar(cab.T0Ms, rf)
			ls, ps, err := pid.Decodificar(a.Geracao, cab, a.Cont, t0, c.pids)
			if err != nil {
				return err
			}
			leituras = append(leituras, ls...)
			posicoes = append(posicoes, ps...)
			tIni, tFim := t0+int64(a.Cont.DtMin), t0+int64(a.Cont.DtMax)
			ref[k] = tFim
			desl := recMs - float64(tFim)
			if x, ok := agg[k]; ok {
				x.seqFim = int64(cab.Seq)
				x.tIni, x.tFim = min(x.tIni, tIni), max(x.tFim, tFim)
				x.desl = min(x.desl, desl)
			} else {
				agg[k] = &aggBoot{seqIni: int64(cab.Seq), seqFim: int64(cab.Seq), tIni: tIni, tFim: tFim, desl: desl}
			}
		}
		brutos[i] = ding.BlocoBruto{Seq: int64(cab.Seq), BootID: int32(cab.BootID), Geracao: int32(a.Geracao), T0Ms: t0,
			Marcador: a.Marcador, Payload: a.B.Dados, Assinatura: a.B.Tag}
	}
	gravados, err := rp.Blocos.InserirBrutos(ctx, d.ID, int64(sessao.CS), agora, brutos)
	if err != nil {
		return err
	}
	if gravados != len(brutos) {
		det, _ := json.Marshal(map[string]any{"esperado": len(brutos), "gravado": gravados})
		_ = rp.Ocorrencias.InserirAlerta(ctx, ocorrencia.Alerta{DongleID: d.ID, Tipo: ocorrencia.AlertaInvariante, Detalhe: det})
		return ding.ErrInvariante
	}
	if len(leituras) > 0 {
		if err := rp.Blocos.CopiarLeituras(ctx, d.ID, agora, leituras); err != nil {
			return err
		}
	}
	if len(posicoes) > 0 {
		if err := rp.Blocos.CopiarPosicoes(ctx, d.ID, agora, posicoes); err != nil {
			return err
		}
	}
	c.met.Linhas("leitura", len(leituras))
	c.met.Linhas("posicao", len(posicoes))
	for _, l := range r.Lacunas {
		sb := int64(l.SeqBloco)
		if err := rp.Ocorrencias.InserirLacuna(ctx, ocorrencia.Lacuna{DongleID: d.ID, From: int64(l.From), To: int64(l.To),
			Causa: l.Causa, RegistradoEm: agora, SeqBloco: &sb}); err != nil {
			return err
		}
		c.met.Lacuna(l.Causa)
	}
	if int32(r.G) > est.GeracaoBoot {
		det, _ := json.Marshal(map[string]any{"de": est.GeracaoBoot, "para": r.G, "ultimo_boot_id": est.UltimoBootID})
		if err := rp.Ocorrencias.InserirAlerta(ctx, ocorrencia.Alerta{DongleID: d.ID, Tipo: ocorrencia.AlertaGeracaoAvancou, Detalhe: det}); err != nil {
			return err
		}
	}
	if len(agg) > 0 {
		chaves := make([]chaveBoot, 0, len(agg))
		for k := range agg {
			chaves = append(chaves, k)
		}
		sort.Slice(chaves, func(i, j int) bool {
			if chaves[i].g != chaves[j].g {
				return chaves[i].g < chaves[j].g
			}
			return chaves[i].b < chaves[j].b
		})
		ags := make([]ding.AgregadoBoot, 0, len(chaves))
		for _, k := range chaves {
			x := agg[k]
			ags = append(ags, ding.AgregadoBoot{Geracao: k.g, BootID: k.b, SeqIni: x.seqIni, SeqFim: x.seqFim,
				TIni: x.tIni, TFim: x.tFim, DeslMax: x.desl})
		}
		if err := rp.Boots.AtualizarIngestao(ctx, d.ID, agora, ags); err != nil {
			return err
		}
		if err := rp.Boots.NotificarPendente(ctx, fmt.Sprintf("%d", d.ID)); err != nil {
			return err
		}
	}
	return nil
}
