package reprocessamento

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"tccelta/backend/internal/banco"
	"tccelta/backend/internal/banco/consultas"
	"tccelta/backend/internal/dominio/pid"
	"tccelta/backend/internal/dominio/protocolo"
	"tccelta/backend/internal/observabilidade/metricas"
)

// OpcoesReprocesso são os parâmetros de `tccelta worker reprocessar`.
type OpcoesReprocesso struct {
	DongleID int32
	DesdeSeq int64
	AteSeq   int64
	Lote     int32
	Aplicar  bool
	Operador string
}

// ResumoReprocesso é o resultado do reprocessamento (ou do ensaio).
type ResumoReprocesso struct {
	Blocos, LinhasRemovidas, LinhasInseridas, ValoresMudariam int64
	UltimoSeq                                                 int64
}

// Reprocessar refaz leitura/posicao a partir de bloco_bruto. Sem Aplicar, é só um ensaio.
// Deve rodar com o papel dono (DELETE em leitura/posicao); não usa o Vault.
func Reprocessar(ctx context.Context, pool *pgxpool.Pool, cat pid.Consulta, o OpcoesReprocesso, log *slog.Logger, met *metricas.M) (ResumoReprocesso, error) {
	var res ResumoReprocesso
	if o.Lote <= 0 {
		o.Lote = 200
	}
	if o.AteSeq <= 0 {
		o.AteSeq = 4294967295
	}
	if o.DesdeSeq <= 0 {
		o.DesdeSeq = 1
	}
	ini := o.DesdeSeq
	q0 := consultas.New(pool)
	for ini <= o.AteSeq {
		blocos, err := q0.ListarBlocosDados(ctx, o.DongleID, ini, o.AteSeq, o.Lote)
		if err != nil {
			return res, err
		}
		if len(blocos) == 0 {
			break
		}
		a, b := blocos[0].Seq, blocos[len(blocos)-1].Seq
		minR, maxR := blocos[0].RecebidoEm, blocos[0].RecebidoEm
		for _, x := range blocos {
			if x.RecebidoEm.Before(minR) {
				minR = x.RecebidoEm
			}
			if x.RecebidoEm.After(maxR) {
				maxR = x.RecebidoEm
			}
		}
		var ls []consultas.CopiarLeiturasParams
		var ps []consultas.CopiarPosicoesParams
		for _, x := range blocos {
			blk, err := protocolo.DecodificarBloco(append(append([]byte{}, x.Payload...), make([]byte, 0)...), protocolo.Perfil{Nome: "raw"})
			if err != nil {
				return res, fmt.Errorf("seq %d: %w", x.Seq, err)
			}
			c, err := protocolo.LerRegistros(blk.Dados)
			if err != nil {
				return res, fmt.Errorf("seq %d: %w", x.Seq, err)
			}
			l, p, err := pid.Decodificar(uint16(x.GeracaoBoot), blk.Cab, c, x.T0Ms, cat)
			if err != nil {
				return res, fmt.Errorf("seq %d: %w (nada foi alterado nesta janela)", x.Seq, err)
			}
			for _, y := range l {
				ls = append(ls, consultas.CopiarLeiturasParams{DongleID: o.DongleID, GeracaoBoot: y.Geracao, BootID: y.BootID, TMs: y.TMs,
					Pid: y.PID, ValorBruto: y.ValorBruto, Valor: y.Valor, SeqBloco: y.SeqBloco, RecebidoEm: x.RecebidoEm})
			}
			for _, y := range p {
				ps = append(ps, consultas.CopiarPosicoesParams{DongleID: o.DongleID, GeracaoBoot: y.Geracao, BootID: y.BootID, TMs: y.TMs,
					Geom: y.Geom.EWKB(), Origem: "app", SeqBloco: y.SeqBloco, RecebidoEm: x.RecebidoEm})
			}
		}
		res.Blocos += int64(len(blocos))
		res.UltimoSeq = b
		if !o.Aplicar {
			n, err := q0.ContarLeituras(ctx, o.DongleID, a, b)
			if err != nil {
				return res, err
			}
			res.LinhasRemovidas += n
			res.LinhasInseridas += int64(len(ls))
		} else {
			var rem int64
			err = nil
			for tent := 0; tent < 5; tent++ {
				err = banco.EmTransacao(ctx, pool, banco.OpcoesPadrao, func(ctx context.Context, q *consultas.Queries, tx pgx.Tx) error {
					if err := q.TravarDongle(ctx, o.DongleID); err != nil {
						return err
					}
					n1, err := q.ApagarLeiturasDoBloco(ctx, consultas.ApagarLeiturasDoBlocoParams{DongleID: o.DongleID, SeqIni: a, SeqFim: b, RecebidoMin: minR, RecebidoMax: maxR})
					if err != nil {
						return err
					}
					if _, err := q.ApagarPosicoesDoBloco(ctx, consultas.ApagarPosicoesDoBlocoParams{DongleID: o.DongleID, SeqIni: a, SeqFim: b, RecebidoMin: minR, RecebidoMax: maxR}); err != nil {
						return err
					}
					if len(ls) > 0 {
						if _, err := q.CopiarLeituras(ctx, ls); err != nil {
							return err
						}
					}
					if len(ps) > 0 {
						if _, err := q.CopiarPosicoes(ctx, ps); err != nil {
							return err
						}
					}
					rem = n1
					return q.MarcarBootsGold(ctx, o.DongleID, &b, &a)
				})
				if !errors.Is(err, banco.ErrBancoOcupado) {
					break
				}
			}
			if err != nil {
				return res, err
			}
			res.LinhasRemovidas += rem
			res.LinhasInseridas += int64(len(ls))
			if met != nil {
				met.Reprocessadas.Add(float64(len(ls)))
			}
		}
		ini = b + 1
	}
	if o.Aplicar {
		det, _ := json.Marshal(map[string]any{"desde_seq": o.DesdeSeq, "ate_seq": o.AteSeq, "blocos": res.Blocos,
			"linhas_removidas": res.LinhasRemovidas, "linhas_inseridas": res.LinhasInseridas, "aplicado": true, "operador": o.Operador})
		if err := q0.InserirAlerta(ctx, consultas.InserirAlertaParams{DongleID: o.DongleID, Tipo: "reprocessamento", Detalhe: det}); err != nil {
			return res, err
		}
		log.Info("ingestao.reprocessado", "dongle_id", o.DongleID, "blocos", res.Blocos, "inseridas", res.LinhasInseridas)
	}
	return res, nil
}
