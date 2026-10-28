package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"tccelta/backend/internal/banco/consultas"
	"tccelta/backend/internal/dominio/dongle"
	"tccelta/backend/internal/dominio/protocolo"
)

type dongles struct{ q consultas.Querier }

func (r *dongles) BuscarPorDevID(ctx context.Context, dev protocolo.DevID) (dongle.Dongle, error) {
	row, err := r.q.BuscarDonglePorDevId(ctx, dev[:])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return dongle.Dongle{}, dongle.ErrDesconhecido
		}
		return dongle.Dongle{}, err
	}
	p, err := protocolo.PerfilPorNome(row.PerfilSeguranca)
	if err != nil {
		return dongle.Dongle{}, err
	}
	return dongle.Dongle{ID: row.DongleID, DevID: dev, Perfil: p, ChaveAuth: row.KmsChaveAuth, ChaveRec: row.KmsChaveRec,
		ChavePublica: row.ChavePublica, Ativo: row.Ativo}, nil
}

func (r *dongles) Travar(ctx context.Context, dongleID int32) error {
	return r.q.TravarDongle(ctx, dongleID)
}
