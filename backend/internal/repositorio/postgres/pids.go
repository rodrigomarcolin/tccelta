package postgres

import (
	"context"

	"tccelta/backend/internal/banco/consultas"
	"tccelta/backend/internal/dominio/pid"
)

type pids struct{ q consultas.Querier }

func (r *pids) Listar(ctx context.Context) ([]pid.Def, error) {
	rows, err := r.q.ListarPids(ctx)
	if err != nil {
		return nil, err
	}
	defs := make([]pid.Def, 0, len(rows))
	for _, x := range rows {
		defs = append(defs, pid.Def{PID: uint8(x.Pid), Nome: x.Nome, Unidade: x.Unidade, NBytes: int(x.NBytes),
			Fator: x.Fator, Deslocamento: x.Deslocamento, Min: x.ValorMin, Max: x.ValorMax})
	}
	return defs, nil
}
