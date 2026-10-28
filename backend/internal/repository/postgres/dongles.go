package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"tccelta/backend/internal/database/dbgen"
	"tccelta/backend/internal/domain/dongle"
	"tccelta/backend/internal/domain/protocol"
)

type dongles struct{ q dbgen.Querier }

func (r *dongles) FindByDevID(ctx context.Context, dev protocol.DevID) (dongle.Dongle, error) {
	row, err := r.q.FindDongleByDevID(ctx, dev[:])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return dongle.Dongle{}, dongle.ErrUnknown
		}
		return dongle.Dongle{}, err
	}
	p, err := protocol.ProfileByName(row.SecurityProfile)
	if err != nil {
		return dongle.Dongle{}, err
	}
	return dongle.Dongle{ID: row.DongleID, DevID: dev, Profile: p, AuthKey: row.KmsAuthKey, RecKey: row.KmsRecKey,
		PublicKey: row.PublicKey, Active: row.Active}, nil
}

func (r *dongles) Lock(ctx context.Context, dongleID int32) error {
	return r.q.LockDongle(ctx, dongleID)
}
