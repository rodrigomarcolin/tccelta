package postgres

import (
	"context"

	"tccelta/backend/internal/banco/consultas"
	"tccelta/backend/internal/dominio/ocorrencia"
)

type ocorrencias struct{ q consultas.Querier }

func (r *ocorrencias) InserirLacuna(ctx context.Context, l ocorrencia.Lacuna) error {
	return r.q.InserirLacuna(ctx, consultas.InserirLacunaParams{DongleID: l.DongleID, FromSeq: l.From, ToSeq: l.To,
		Causa: l.Causa, RegistradoEm: l.RegistradoEm, CS: l.CS, SeqBloco: l.SeqBloco})
}

func (r *ocorrencias) InserirAlerta(ctx context.Context, a ocorrencia.Alerta) error {
	return r.q.InserirAlerta(ctx, consultas.InserirAlertaParams{DongleID: a.DongleID, GeracaoBoot: a.GeracaoBoot, BootID: a.BootID,
		Tipo: a.Tipo, Detalhe: a.Detalhe})
}
