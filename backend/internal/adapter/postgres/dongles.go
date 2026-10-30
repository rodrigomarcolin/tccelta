package postgres

import (
	"context"
	"errors"
	"fmt"

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

func (r *dongles) Register(ctx context.Context, g dongle.Registration) (int32, error) {
	var desc *string
	if g.Description != "" {
		d := g.Description
		desc = &d
	}
	id, err := r.q.InsertDongle(ctx, dbgen.InsertDongleParams{DevID: g.DevID[:], SecurityProfile: g.Profile,
		KmsAuthKey: g.AuthKey, KmsRecKey: g.RecKey, PublicKey: g.PublicKey, Description: desc})
	if err != nil {
		return 0, fmt.Errorf("cadastro: %w", err)
	}
	if err := r.q.InsertDongleState(ctx, id); err != nil {
		return 0, err
	}
	return id, nil
}
