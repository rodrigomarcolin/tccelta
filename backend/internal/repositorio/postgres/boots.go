package postgres

import (
	"context"
	"time"

	"tccelta/backend/internal/banco/consultas"
	"tccelta/backend/internal/dominio/handshake"
	"tccelta/backend/internal/dominio/ingestao"
	"tccelta/backend/internal/dominio/tempo"
)

type boots struct{ q consultas.Querier }

func (r *boots) TMsUltimo(ctx context.Context, dongleID int32, bootIDs []int32, gMin, gMax int32) ([]tempo.RefBoot, error) {
	rows, err := r.q.LerTMsUltimoBoots(ctx, dongleID, bootIDs, gMin, gMax)
	if err != nil {
		return nil, err
	}
	out := make([]tempo.RefBoot, 0, len(rows))
	for _, x := range rows {
		if x.TMsUltimo != nil {
			out = append(out, tempo.RefBoot{Geracao: x.GeracaoBoot, BootID: x.BootID, TMsUltimo: *x.TMsUltimo})
		}
	}
	return out, nil
}

func (r *boots) GarantirPendente(ctx context.Context, dongleID, geracao, bootID int32, agora time.Time) error {
	return r.q.GarantirBootPendente(ctx, dongleID, geracao, bootID, agora)
}

func (r *boots) AtualizarIngestao(ctx context.Context, dongleID int32, agora time.Time, ags []ingestao.AgregadoBoot) error {
	p := consultas.AtualizarBootsIngestaoParams{DongleID: dongleID, Agora: agora}
	for _, x := range ags {
		p.Geracoes = append(p.Geracoes, x.Geracao)
		p.BootIds = append(p.BootIds, x.BootID)
		p.SeqInis = append(p.SeqInis, x.SeqIni)
		p.SeqFims = append(p.SeqFims, x.SeqFim)
		p.TInis = append(p.TInis, x.TIni)
		p.TFims = append(p.TFims, x.TFim)
		p.DeslMaxs = append(p.DeslMaxs, x.DeslMax)
	}
	return r.q.AtualizarBootsIngestao(ctx, p)
}

func (r *boots) InserirAncora(ctx context.Context, a handshake.Ancora) error {
	return r.q.InserirAncora(ctx, consultas.InserirAncoraParams{DongleID: a.DongleID, GeracaoBoot: a.GeracaoBoot, BootID: a.BootID,
		TMs: a.TMs, Nonce: a.Nonce, S1: a.S1, S2: a.S2})
}

func (r *boots) NotificarPendente(ctx context.Context, chave string) error {
	return r.q.NotificarBootPendente(ctx, chave)
}
