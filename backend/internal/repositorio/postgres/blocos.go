package postgres

import (
	"context"
	"time"

	"tccelta/backend/internal/banco/consultas"
	"tccelta/backend/internal/dominio/ingestao"
	"tccelta/backend/internal/dominio/pid"
)

type blocos struct{ q consultas.Querier }

func (r *blocos) InserirBrutos(ctx context.Context, dongleID int32, epoca int64, recebidoEm time.Time, bs []ingestao.BlocoBruto) (int, error) {
	n := len(bs)
	in := consultas.InserirBlocosBrutosParams{DongleID: dongleID, Epoca: epoca, RecebidoEm: recebidoEm,
		Seqs: make([]int64, n), BootIds: make([]int32, n), Geracoes: make([]int32, n), T0s: make([]int64, n),
		Marcadores: make([]string, n), Payloads: make([][]byte, n), Assinaturas: make([][]byte, n)}
	for i, b := range bs {
		in.Seqs[i], in.BootIds[i], in.Geracoes[i], in.T0s[i] = b.Seq, b.BootID, b.Geracao, b.T0Ms
		in.Marcadores[i], in.Payloads[i], in.Assinaturas[i] = b.Marcador, b.Payload, b.Assinatura
	}
	gravados, err := r.q.InserirBlocosBrutos(ctx, in)
	return len(gravados), err
}

func (r *blocos) RegistrarRejeitado(ctx context.Context, x ingestao.Rejeitado) error {
	var det *string
	if x.Detalhe != "" {
		d := x.Detalhe
		det = &d
	}
	seq, boot, t0, jti := x.Seq, x.BootID, x.T0Ms, x.TokenJTI
	return r.q.RegistrarRejeitado(ctx, consultas.RegistrarRejeitadoParams{DongleID: x.DongleID, Seq: &seq, BootID: &boot, T0Ms: &t0,
		Payload: x.Payload, Assinatura: x.Assinatura, Motivo: string(x.Motivo), Detalhe: det, HashConteudo: x.HashConteudo,
		TokenJti: &jti, RecebidoEm: x.RecebidoEm})
}

func (r *blocos) CopiarLeituras(ctx context.Context, dongleID int32, recebidoEm time.Time, ls []pid.Leitura) error {
	ps := make([]consultas.CopiarLeiturasParams, len(ls))
	for i, l := range ls {
		ps[i] = consultas.CopiarLeiturasParams{DongleID: dongleID, GeracaoBoot: l.Geracao, BootID: l.BootID, TMs: l.TMs,
			Pid: l.PID, ValorBruto: l.ValorBruto, Valor: l.Valor, SeqBloco: l.SeqBloco, RecebidoEm: recebidoEm}
	}
	_, err := r.q.CopiarLeituras(ctx, ps)
	return err
}

func (r *blocos) CopiarPosicoes(ctx context.Context, dongleID int32, recebidoEm time.Time, pos []pid.Posicao) error {
	ps := make([]consultas.CopiarPosicoesParams, len(pos))
	for i, p := range pos {
		ps[i] = consultas.CopiarPosicoesParams{DongleID: dongleID, GeracaoBoot: p.Geracao, BootID: p.BootID, TMs: p.TMs,
			Geom: p.Geom.EWKB(), Origem: "app", SeqBloco: p.SeqBloco, RecebidoEm: recebidoEm}
	}
	_, err := r.q.CopiarPosicoes(ctx, ps)
	return err
}
