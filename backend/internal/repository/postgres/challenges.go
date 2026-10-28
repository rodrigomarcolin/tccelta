package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"tccelta/backend/internal/banco/consultas"
	"tccelta/backend/internal/dominio/handshake"
)

type desafios struct{ q consultas.Querier }

func (r *desafios) Emitir(ctx context.Context, e handshake.Emissao) (handshake.Desafio, error) {
	row, err := r.q.EmitirDesafio(ctx, consultas.EmitirDesafioParams{
		AgoraS: e.S1.Unix(), DongleID: e.DongleID, S1: e.S1, MaxPendentes: int32(e.MaxPendentes),
		Nonce: e.Nonce, ExpiraEm: e.ExpiraEm, FlagsC: 0,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return handshake.Desafio{}, handshake.ErrDesafiosPendentes
		}
		return handshake.Desafio{}, err
	}
	return handshake.Desafio{CS: row.CS, SeqContiguo: row.SeqContiguo, CConf: row.CConf}, nil
}

func (r *desafios) Buscar(ctx context.Context, dongleID int32, cs int64) (handshake.DesafioGravado, error) {
	row, err := r.q.BuscarDesafio(ctx, dongleID, cs)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return handshake.DesafioGravado{}, handshake.ErrDesafioInvalido
		}
		return handshake.DesafioGravado{}, err
	}
	return handshake.DesafioGravado{Nonce: row.Nonce, S1: row.S1, ExpiraEm: row.ExpiraEm, RespondidoEm: row.RespondidoEm}, nil
}

func (r *desafios) Consumir(ctx context.Context, c handshake.Consumo) (time.Time, error) {
	s2, boot, tms, head, tail := c.S2, c.BootID, c.TMs, c.HeadSeq, c.TailSeq
	s1, err := r.q.ConsumirDesafio(ctx, consultas.ConsumirDesafioParams{S2: &s2, BootID: &boot, TMs: &tms,
		HeadSeq: &head, TailSeq: &tail, DongleID: c.DongleID, CS: c.CS, Nonce: c.Nonce})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return time.Time{}, handshake.ErrDesafioInvalido
		}
		return time.Time{}, err
	}
	return s1, nil
}

func (r *desafios) Finalizar(ctx context.Context, f handshake.Finalizacao) error {
	res := f.Resultado
	return r.q.FinalizarDesafio(ctx, consultas.FinalizarDesafioParams{Resultado: &res, AckSeq: f.AckSeq,
		PerdaIni: f.PerdaIni, PerdaFim: f.PerdaFim, TokenJti: f.TokenJTI, TMsDesembrulhado: f.TMsDesembrulhado,
		GeracaoBoot: f.GeracaoBoot, DongleID: f.DongleID, CS: f.CS})
}

func (r *desafios) MarcarTagInvalida(ctx context.Context, dongleID int32, cs int64, nonce []byte) error {
	return r.q.MarcarTagInvalida(ctx, dongleID, cs, nonce)
}
