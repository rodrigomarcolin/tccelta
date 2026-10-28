package postgres

import (
	"context"

	"tccelta/backend/internal/database/dbgen"
	"tccelta/backend/internal/domain/pid"
)

type pids struct{ q dbgen.Querier }

func (r *pids) List(ctx context.Context) ([]pid.Def, error) {
	rows, err := r.q.ListPids(ctx)
	if err != nil {
		return nil, err
	}
	defs := make([]pid.Def, 0, len(rows))
	for _, x := range rows {
		defs = append(defs, pid.Def{PID: uint8(x.Pid), Name: x.Name, Unit: x.Unit, NBytes: int(x.NBytes),
			Factor: x.Factor, ValueOffset: x.ValueOffset, Min: x.MinValue, Max: x.MaxValue})
	}
	return defs, nil
}
