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

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"

	"tccelta/backend/internal/banco"
	"tccelta/backend/internal/banco/consultas"
	"tccelta/backend/internal/dongles"
	"tccelta/backend/internal/kms"
	"tccelta/backend/internal/metricas"
	"tccelta/backend/internal/pids"
	"tccelta/backend/internal/protocolo"
	"tccelta/backend/internal/tempo"
	"tccelta/backend/internal/token"
)

// Erros do serviço.
var (
	ErrLoteMalformado = errors.New("lote_malformado")
	ErrLoteGrande     = errors.New("lote_grande")
	ErrSemConfirmacao = errors.New("sem_confirmacao")
	ErrInvariante     = errors.New("invariante_violada")
)

// Servico processa lotes de blocos.
type Servico struct {
	Pool           *pgxpool.Pool
	KMS            kms.Autenticador
	Pids           *pids.Catalogo
	Agora          func() time.Time
	LoteMax        int
	VerifyLote     int
	VerifyParalelo int
	Log            *slog.Logger
	Met            *metricas.M
}

// ResultadoLote é o resultado de ProcessarLote.
type ResultadoLote struct {
	AckTLV       []byte
	AckSeq       uint32
	Interrompido bool
	Motivo       Motivo
	Acao         Acao
	SemProgresso bool
	Aceitos      int
	Duplicados   int
}

func (s *Servico) agora() time.Time {
	if s.Agora != nil {
		return s.Agora()
	}
	return time.Now()
}

func (s *Servico) bloco(resultado string, n int) {
	if s.Met != nil && n > 0 {
		s.Met.Blocos.WithLabelValues(resultado).Add(float64(n))
	}
}

func ackDecodificado(tlv []byte) (protocolo.Ack, bool) {
	t, n, err := protocolo.LerTLV(tlv)
	if err != nil || n != len(tlv) || t.T != protocolo.TipoAck {
		return protocolo.Ack{}, false
	}
	a, _, err := protocolo.DecodificarAck(t.V)
	return a, err == nil
}

// ProcessarLote implementa o algoritmo da spec 05.
func (s *Servico) ProcessarLote(ctx context.Context, d dongles.Dongle, sessao token.Sessao, corpo []byte) (ResultadoLote, error) {
	// 1. forma
	tlvs, err := protocolo.LerLote(corpo, s.LoteMax)
	if err != nil {
		if errors.Is(err, protocolo.ErrLoteGrande) {
			return ResultadoLote{}, ErrLoteGrande
		}
		return ResultadoLote{}, fmt.Errorf("%w: %v", ErrLoteMalformado, err)
	}
	blocos := make([]BlocoIn, len(tlvs))
	for i, t := range tlvs {
		if t.T != protocolo.TipoBloco {
			return ResultadoLote{}, fmt.Errorf("%w: tipo 0x%02x", ErrLoteMalformado, uint8(t.T))
		}
		b, err := protocolo.DecodificarBloco(t.V, d.Perfil)
		if err != nil {
			return ResultadoLote{}, fmt.Errorf("%w: %v", ErrLoteMalformado, err)
		}
		blocos[i].B = b
	}

	// 2. snapshot
	q := consultas.New(s.Pool)
	snap, err := q.LerSnapshotEstado(ctx, d.ID)
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
	g, gctx := errgroup.WithContext(kms.ComFase(ctx, kms.FasePre))
	if len(idx) > 0 {
		lote := s.VerifyLote
		if lote <= 0 {
			lote = 32
		}
		par := s.VerifyParalelo
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
				out, err := s.KMS.VerificarBlocos(gctx, d, ents, tags)
				if err != nil {
					return err
				}
				copy(res[ini:fim], out)
				return nil
			})
		}
		g.Go(func() error {
			// ACK provável: simula com todas as tags válidas sobre o snapshot
			sim := make([]BlocoIn, len(blocos))
			copy(sim, blocos)
			for i := range sim {
				sim[i].Valido = true
			}
			r := aplicar(sim, EstadoIn{SeqContiguo: uint32(snap.SeqContiguo)}, s.Pids)
			prov := protocolo.Ack{AckSeq: r.NovoAck, CConf: uint32(snap.CConf)}
			if a, ok := ackDecodificado(snap.UltimoAck); ok && a == prov {
				return nil
			}
			macs, err := s.KMS.HMACAuth(gctx, d, [][]byte{protocolo.EntradaTag(protocolo.TipoAck, d.DevID, prov.Payload())})
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
	err = banco.EmTransacao(ctx, s.Pool, banco.OpcoesPadrao, func(ctx context.Context, q *consultas.Queries, tx pgx.Tx) error {
		out = ResultadoLote{}
		if err := q.TravarDongle(ctx, d.ID); err != nil {
			return err
		}
		e, err := q.LerEstado(ctx, d.ID)
		if err != nil {
			return err
		}
		agora := s.agora()
		r := aplicar(blocos, EstadoIn{SeqContiguo: uint32(e.SeqContiguo), G: uint16(e.GeracaoBoot), U: uint16(e.UltimoBootID)}, s.Pids)

		for _, rj := range r.Rejeitados {
			if err := s.rejeitar(ctx, q, d, sessao, rj, agora); err != nil {
				return err
			}
			if rj.Motivo == MotivoPIDDesconhecido {
				det, _ := json.Marshal(map[string]any{"pid": rj.PID, "seq": rj.B.Cab.Seq})
				if err := q.InserirAlerta(ctx, consultas.InserirAlertaParams{DongleID: d.ID, Tipo: "pid_desconhecido", Detalhe: det}); err != nil {
					return err
				}
			}
		}

		if len(r.Aceitos) > 0 {
			if err := s.gravar(ctx, q, d, sessao, e, r, agora); err != nil {
				return err
			}
		}

		ackAtual, err := q.LerUltimoAck(ctx, d.ID)
		if err != nil {
			return err
		}
		ack := protocolo.Ack{AckSeq: r.NovoAck, CConf: uint32(e.CConf)}
		var ackTLV []byte
		if a, ok := ackDecodificado(ackAtual); ok && a == ack {
			ackTLV = ackAtual
		} else if preAssinado && ackPre == ack {
			ackTLV = ack.Montar(tagPre)
		} else {
			macs, err := s.KMS.HMACAuth(kms.ComFase(ctx, kms.FaseTx), d,
				[][]byte{protocolo.EntradaTag(protocolo.TipoAck, d.DevID, ack.Payload())})
			if err != nil {
				return err
			}
			ackTLV = ack.Montar(macs[0][:protocolo.TamTagAuth])
		}
		if int64(r.NovoAck) != e.SeqContiguo || int64(r.MaiorVisto) > e.MaiorSeqVisto || int64(r.G) != int64(e.GeracaoBoot) ||
			!bytesIguais(ackTLV, ackAtual) {
			ag := agora
			if err := q.AtualizarEstadoIngestao(ctx, consultas.AtualizarEstadoIngestaoParams{
				SeqContiguo: int64(r.NovoAck), MaiorVisto: int64(r.MaiorVisto), UltimoAck: ackTLV, Agora: &ag,
				GeracaoBoot: int32(r.G), UltimoBootID: int32(r.U), DongleID: d.ID}); err != nil {
				return err
			}
		}
		out = ResultadoLote{AckTLV: ackTLV, AckSeq: r.NovoAck, Motivo: r.Motivo, Acao: AcaoDe(r.Motivo),
			Aceitos: len(r.Aceitos), Duplicados: r.Duplicados}
		ultimo := blocos[len(blocos)-1].Seq()
		out.Interrompido = r.NovoAck < ultimo
		out.SemProgresso = int64(r.NovoAck) == e.SeqContiguo && r.Motivo == MotivoTagInvalida
		s.contar(r)
		return nil
	})
	if s.Met != nil {
		s.Met.IngestaoTx.Observe(time.Since(t0tx).Seconds())
	}
	if err != nil {
		return ResultadoLote{}, err
	}
	if s.Met != nil {
		st := "completo"
		if out.SemProgresso {
			st = "sem_progresso"
		} else if out.Interrompido {
			st = "interrompido"
		}
		s.Met.Lotes.WithLabelValues(st).Inc()
	}
	if out.Interrompido {
		s.Log.Warn("lote.interrompido", "dongle", d.DevID.Hex(), "motivo", string(out.Motivo), "ack_seq", out.AckSeq)
	}
	return out, nil
}

func bytesIguais(a, b []byte) bool { return string(a) == string(b) }

func (s *Servico) contar(r ResultadoAplicacao) {
	s.bloco("aceito", len(r.Aceitos))
	s.bloco("duplicado", r.Duplicados)
	for _, rj := range r.Rejeitados {
		s.bloco(string(rj.Motivo), 1)
	}
	if r.Motivo == MotivoSalto {
		s.bloco("descartado_apos_interrupcao", 1)
	}
}

func (s *Servico) rejeitar(ctx context.Context, q *consultas.Queries, d dongles.Dongle, sessao token.Sessao, rj Rejeicao, agora time.Time) error {
	h := sha256.Sum256(append(append([]byte{}, rj.B.Dados...), rj.B.Tag...))
	seq, boot, t0 := int64(rj.B.Cab.Seq), int32(rj.B.Cab.BootID), int64(rj.B.Cab.T0Ms)
	jti := sessao.JTI
	var det *string
	if rj.Detalhe != "" {
		det = &rj.Detalhe
	}
	return q.RegistrarRejeitado(ctx, consultas.RegistrarRejeitadoParams{DongleID: d.ID, Seq: &seq, BootID: &boot, T0Ms: &t0,
		Payload: rj.B.Dados, Assinatura: rj.B.Tag, Motivo: string(rj.Motivo), Detalhe: det, HashConteudo: h[:],
		TokenJti: &jti, RecebidoEm: agora})
}

type chaveBoot struct{ g, b int32 }

type aggBoot struct {
	seqIni, seqFim, tIni, tFim int64
	desl                       float64
}

// gravar persiste os blocos aceitos e tudo que deriva deles.
func (s *Servico) gravar(ctx context.Context, q *consultas.Queries, d dongles.Dongle, sessao token.Sessao,
	e consultas.LerEstadoRow, r ResultadoAplicacao, agora time.Time) error {
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
		rows, err := q.LerTMsUltimoBoots(ctx, d.ID, ids, e.GeracaoBoot, e.GeracaoBoot+1)
		if err != nil {
			return err
		}
		for _, x := range rows {
			if x.TMsUltimo != nil {
				ref[chaveBoot{x.GeracaoBoot, x.BootID}] = *x.TMsUltimo
			}
		}
	}
	n := len(r.Aceitos)
	in := consultas.InserirBlocosBrutosParams{DongleID: d.ID, Epoca: int64(sessao.CS), RecebidoEm: agora,
		Seqs: make([]int64, n), BootIds: make([]int32, n), Geracoes: make([]int32, n), T0s: make([]int64, n),
		Marcadores: make([]string, n), Payloads: make([][]byte, n), Assinaturas: make([][]byte, n)}
	var leituras []consultas.CopiarLeiturasParams
	var posicoes []consultas.CopiarPosicoesParams
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
			ls, ps, err := Decodificar(a.Geracao, cab, a.Cont, t0, s.Pids)
			if err != nil {
				return err
			}
			for _, l := range ls {
				leituras = append(leituras, consultas.CopiarLeiturasParams{DongleID: d.ID, GeracaoBoot: l.Geracao, BootID: l.BootID,
					TMs: l.TMs, Pid: l.PID, ValorBruto: l.ValorBruto, Valor: l.Valor, SeqBloco: l.SeqBloco, RecebidoEm: agora})
			}
			for _, p := range ps {
				posicoes = append(posicoes, consultas.CopiarPosicoesParams{DongleID: d.ID, GeracaoBoot: p.Geracao, BootID: p.BootID,
					TMs: p.TMs, Geom: p.Geom.EWKB(), Origem: "app", SeqBloco: p.SeqBloco, RecebidoEm: agora})
			}
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
		in.Seqs[i], in.BootIds[i], in.Geracoes[i], in.T0s[i] = int64(cab.Seq), int32(cab.BootID), int32(a.Geracao), t0
		in.Marcadores[i], in.Payloads[i], in.Assinaturas[i] = a.Marcador, a.B.Dados, a.B.Tag
	}
	gravados, err := q.InserirBlocosBrutos(ctx, in)
	if err != nil {
		return err
	}
	if len(gravados) != n {
		det, _ := json.Marshal(map[string]any{"esperado": n, "gravado": len(gravados)})
		_ = q.InserirAlerta(ctx, consultas.InserirAlertaParams{DongleID: d.ID, Tipo: "invariante_violada", Detalhe: det})
		return ErrInvariante
	}
	if len(leituras) > 0 {
		if _, err := q.CopiarLeituras(ctx, leituras); err != nil {
			return err
		}
	}
	if len(posicoes) > 0 {
		if _, err := q.CopiarPosicoes(ctx, posicoes); err != nil {
			return err
		}
	}
	if s.Met != nil {
		s.Met.Linhas.WithLabelValues("leitura").Add(float64(len(leituras)))
		s.Met.Linhas.WithLabelValues("posicao").Add(float64(len(posicoes)))
	}
	for _, l := range r.Lacunas {
		sb, from, to := int64(l.SeqBloco), int64(l.From), int64(l.To)
		if err := q.InserirLacuna(ctx, consultas.InserirLacunaParams{DongleID: d.ID, FromSeq: from, ToSeq: to,
			Causa: l.Causa, RegistradoEm: agora, SeqBloco: &sb}); err != nil {
			return err
		}
		if s.Met != nil {
			s.Met.Lacunas.WithLabelValues(l.Causa).Inc()
		}
	}
	if int32(r.G) > e.GeracaoBoot {
		det, _ := json.Marshal(map[string]any{"de": e.GeracaoBoot, "para": r.G, "ultimo_boot_id": e.UltimoBootID})
		if err := q.InserirAlerta(ctx, consultas.InserirAlertaParams{DongleID: d.ID, Tipo: "geracao_boot_avancou", Detalhe: det}); err != nil {
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
		p := consultas.AtualizarBootsIngestaoParams{DongleID: d.ID, Agora: agora}
		for _, k := range chaves {
			x := agg[k]
			p.Geracoes = append(p.Geracoes, k.g)
			p.BootIds = append(p.BootIds, k.b)
			p.SeqInis = append(p.SeqInis, x.seqIni)
			p.SeqFims = append(p.SeqFims, x.seqFim)
			p.TInis = append(p.TInis, x.tIni)
			p.TFims = append(p.TFims, x.tFim)
			p.DeslMaxs = append(p.DeslMaxs, x.desl)
		}
		if err := q.AtualizarBootsIngestao(ctx, p); err != nil {
			return err
		}
		if err := q.NotificarBootPendente(ctx, fmt.Sprintf("%d", d.ID)); err != nil {
			return err
		}
	}
	return nil
}

// UltimaConfirmacao devolve o último ACK gravado, sem chamar o KMS.
func (s *Servico) UltimaConfirmacao(ctx context.Context, d dongles.Dongle) ([]byte, error) {
	ack, err := consultas.New(s.Pool).LerUltimoAck(ctx, d.ID)
	if err != nil {
		return nil, err
	}
	if len(ack) == 0 {
		return nil, ErrSemConfirmacao
	}
	return ack, nil
}
