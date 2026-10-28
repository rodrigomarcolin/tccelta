package reprocessing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"tccelta/backend/internal/database"
	"tccelta/backend/internal/database/dbgen"
	"tccelta/backend/internal/domain/pid"
	"tccelta/backend/internal/domain/protocol"
	"tccelta/backend/internal/observability/metrics"
)

// ReprocessOptions são os parâmetros de `tccelta worker reprocess`.
type ReprocessOptions struct {
	DongleID int32
	FromSeq  int64
	ToSeq    int64
	Batch    int32
	Apply    bool
	Operator string
}

// ReprocessSummary é o resultado do reprocessamento (ou do ensaio).
type ReprocessSummary struct {
	Blocks, RemovedRows, InsertedRows, ValuesWouldChange int64
	LastSeq                                              int64
}

// Reprocess refaz reading/position a partir de raw_block. Sem Apply, é só um ensaio.
// Deve rodar com o papel tccelta_owner (DELETE em reading/position); não usa o Vault.
func Reprocess(ctx context.Context, pool *pgxpool.Pool, cat pid.Query, o ReprocessOptions, log *slog.Logger, met *metrics.M) (ReprocessSummary, error) {
	var res ReprocessSummary
	if o.Batch <= 0 {
		o.Batch = 200
	}
	if o.ToSeq <= 0 {
		o.ToSeq = 4294967295
	}
	if o.FromSeq <= 0 {
		o.FromSeq = 1
	}
	start := o.FromSeq
	q0 := dbgen.New(pool)
	for start <= o.ToSeq {
		blocks, err := q0.ListDataBlocks(ctx, o.DongleID, start, o.ToSeq, o.Batch)
		if err != nil {
			return res, err
		}
		if len(blocks) == 0 {
			break
		}
		a, b := blocks[0].Seq, blocks[len(blocks)-1].Seq
		minR, maxR := blocks[0].ReceivedAt, blocks[0].ReceivedAt
		for _, x := range blocks {
			if x.ReceivedAt.Before(minR) {
				minR = x.ReceivedAt
			}
			if x.ReceivedAt.After(maxR) {
				maxR = x.ReceivedAt
			}
		}
		var ls []dbgen.CopyReadingsParams
		var ps []dbgen.CopyPositionsParams
		for _, x := range blocks {
			blk, err := protocol.DecodeBlock(append(append([]byte{}, x.Payload...), make([]byte, 0)...), protocol.Profile{Name: "raw"})
			if err != nil {
				return res, fmt.Errorf("seq %d: %w", x.Seq, err)
			}
			c, err := protocol.ReadRecords(blk.Data)
			if err != nil {
				return res, fmt.Errorf("seq %d: %w", x.Seq, err)
			}
			l, p, err := pid.Decode(uint16(x.BootGeneration), blk.Hdr, c, x.T0Ms, cat)
			if err != nil {
				return res, fmt.Errorf("seq %d: %w (nada foi alterado nesta janela)", x.Seq, err)
			}
			for _, y := range l {
				ls = append(ls, dbgen.CopyReadingsParams{DongleID: o.DongleID, BootGeneration: y.Generation, BootID: y.BootID, TMs: y.TMs,
					Pid: y.PID, RawValue: y.RawValue, Value: y.Value, BlockSeq: y.BlockSeq, ReceivedAt: x.ReceivedAt})
			}
			for _, y := range p {
				ps = append(ps, dbgen.CopyPositionsParams{DongleID: o.DongleID, BootGeneration: y.Generation, BootID: y.BootID, TMs: y.TMs,
					Geom: y.Geom.EWKB(), Source: "app", BlockSeq: y.BlockSeq, ReceivedAt: x.ReceivedAt})
			}
		}
		res.Blocks += int64(len(blocks))
		res.LastSeq = b
		if !o.Apply {
			n, err := q0.CountReadings(ctx, o.DongleID, a, b)
			if err != nil {
				return res, err
			}
			res.RemovedRows += n
			res.InsertedRows += int64(len(ls))
		} else {
			var rem int64
			err = nil
			for attempt := 0; attempt < 5; attempt++ {
				err = database.InTransaction(ctx, pool, database.DefaultOptions, func(ctx context.Context, q *dbgen.Queries, tx pgx.Tx) error {
					if err := q.LockDongle(ctx, o.DongleID); err != nil {
						return err
					}
					n1, err := q.DeleteBlockReadings(ctx, dbgen.DeleteBlockReadingsParams{DongleID: o.DongleID, FromSeq: a, ToSeq: b, ReceivedMin: minR, ReceivedMax: maxR})
					if err != nil {
						return err
					}
					if _, err := q.DeleteBlockPositions(ctx, dbgen.DeleteBlockPositionsParams{DongleID: o.DongleID, FromSeq: a, ToSeq: b, ReceivedMin: minR, ReceivedMax: maxR}); err != nil {
						return err
					}
					if len(ls) > 0 {
						if _, err := q.CopyReadings(ctx, ls); err != nil {
							return err
						}
					}
					if len(ps) > 0 {
						if _, err := q.CopyPositions(ctx, ps); err != nil {
							return err
						}
					}
					rem = n1
					return q.MarkGoldBoots(ctx, o.DongleID, &b, &a)
				})
				if !errors.Is(err, database.ErrDatabaseBusy) {
					break
				}
			}
			if err != nil {
				return res, err
			}
			res.RemovedRows += rem
			res.InsertedRows += int64(len(ls))
			if met != nil {
				met.Reprocessed.Add(float64(len(ls)))
			}
		}
		start = b + 1
	}
	if o.Apply {
		det, _ := json.Marshal(map[string]any{"from_seq": o.FromSeq, "to_seq": o.ToSeq, "blocks": res.Blocks,
			"removed_rows": res.RemovedRows, "inserted_rows": res.InsertedRows, "applied": true, "operator": o.Operator})
		if err := q0.InsertAlert(ctx, dbgen.InsertAlertParams{DongleID: o.DongleID, Type: "reprocessing", Detail: det}); err != nil {
			return res, err
		}
		log.Info("ingestao.reprocessado", "dongle_id", o.DongleID, "blocos", res.Blocks, "inseridas", res.InsertedRows)
	}
	return res, nil
}
