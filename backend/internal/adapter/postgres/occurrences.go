package postgres

import (
	"context"

	"tccelta/backend/internal/database/dbgen"
	"tccelta/backend/internal/domain/occurrence"
)

type occurrences struct{ q dbgen.Querier }

func (r *occurrences) InsertGap(ctx context.Context, l occurrence.Gap) error {
	return r.q.InsertGap(ctx, dbgen.InsertGapParams{DongleID: l.DongleID, FromSeq: l.From, ToSeq: l.To,
		Cause: l.Cause, RegisteredAt: l.RegisteredAt, CS: l.CS, BlockSeq: l.BlockSeq})
}

func (r *occurrences) InsertAlert(ctx context.Context, a occurrence.Alert) error {
	return r.q.InsertAlert(ctx, dbgen.InsertAlertParams{DongleID: a.DongleID, BootGeneration: a.BootGeneration, BootID: a.BootID,
		Type: a.Type, Detail: a.Detail})
}
