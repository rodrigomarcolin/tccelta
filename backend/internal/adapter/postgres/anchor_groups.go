package postgres

import (
	"context"
	"time"

	"tccelta/backend/internal/database/dbgen"
	"tccelta/backend/internal/domain/timeline"
)

type anchorGroups struct{ q dbgen.Querier }

func (r *anchorGroups) Delete(ctx context.Context, k timeline.BootKey) error {
	return r.q.DeleteAnchorGroups(ctx, k.DongleID, k.Generation, k.BootID)
}

func (r *anchorGroups) Copy(ctx context.Context, k timeline.BootKey, computedAt time.Time, groups []timeline.Group) error {
	rows := make([]dbgen.CopyAnchorGroupsParams, len(groups))
	for i, g := range groups {
		rows[i] = dbgen.CopyAnchorGroupsParams{DongleID: k.DongleID, BootGeneration: k.Generation, BootID: k.BootID,
			Ordinal: int32(i + 1), FirstTMs: g.FirstT, LastTMs: g.LastT, TCenterMs: g.TCenter, LoMs: g.Lo, HiMs: g.Hi,
			NAnchors: int32(g.N), ComputedAt: computedAt}
	}
	_, err := r.q.CopyAnchorGroups(ctx, rows)
	return err
}
