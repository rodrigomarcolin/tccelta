package postgres

import (
	"context"

	"tccelta/backend/internal/banco/consultas"
	"tccelta/backend/internal/dominio/dongle"
	"tccelta/backend/internal/dominio/handshake"
	"tccelta/backend/internal/dominio/ingestao"
)

type estados struct{ q consultas.Querier }

func (r *estados) Ler(ctx context.Context, dongleID int32) (dongle.Estado, error) {
	e, err := r.q.LerEstado(ctx, dongleID)
	if err != nil {
		return dongle.Estado{}, err
	}
	return dongle.Estado{SeqContiguo: e.SeqContiguo, MaiorSeqVisto: e.MaiorSeqVisto, HeadSeq: e.HeadSeq, CConf: e.CConf,
		GeracaoBoot: e.GeracaoBoot, UltimoBootID: e.UltimoBootID}, nil
}

func (r *estados) LerSnapshot(ctx context.Context, dongleID int32) (dongle.Estado, error) {
	e, err := r.q.LerSnapshotEstado(ctx, dongleID)
	if err != nil {
		return dongle.Estado{}, err
	}
	return dongle.Estado{SeqContiguo: e.SeqContiguo, CConf: e.CConf, UltimoAck: e.UltimoAck}, nil
}

func (r *estados) LerUltimoAck(ctx context.Context, dongleID int32) ([]byte, error) {
	return r.q.LerUltimoAck(ctx, dongleID)
}

func (r *estados) AtualizarHandshake(ctx context.Context, a handshake.Atualizacao) error {
	tail, s2, agora := a.TailSeq, a.S2, a.Agora
	return r.q.AtualizarEstadoHandshake(ctx, consultas.AtualizarEstadoHandshakeParams{
		SeqContiguo: a.SeqContiguo, HeadSeq: a.HeadSeq, TailSeq: &tail, S2: &s2, CConf: a.CConf, UltimoAck: a.UltimoAck,
		Agora: &agora, DongleID: a.DongleID})
}

func (r *estados) AtualizarIngestao(ctx context.Context, a ingestao.AtualizacaoEstado) error {
	agora := a.Agora
	return r.q.AtualizarEstadoIngestao(ctx, consultas.AtualizarEstadoIngestaoParams{
		SeqContiguo: a.SeqContiguo, MaiorVisto: a.MaiorVisto, UltimoAck: a.UltimoAck, Agora: &agora,
		GeracaoBoot: a.GeracaoBoot, UltimoBootID: a.UltimoBootID, DongleID: a.DongleID})
}
