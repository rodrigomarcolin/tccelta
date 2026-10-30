// Package reprocess refaz reading/position a partir de bronze.raw_block (`tccelta worker reprocess`). Depende só do
// domínio e das portas. Deve rodar com o papel tccelta_owner (DELETE em reading/position); não usa o Vault.
package reprocess

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"tccelta/backend/internal/domain/occurrence"
	"tccelta/backend/internal/domain/pid"
	"tccelta/backend/internal/domain/protocol"
	"tccelta/backend/internal/port"
)

// Options são os parâmetros de `tccelta worker reprocess`.
type Options struct {
	DongleID int32
	FromSeq  int64
	ToSeq    int64
	Batch    int32
	Apply    bool
	Operator string
}

// Summary é o resultado do reprocessamento (ou do ensaio).
type Summary struct {
	Blocks, RemovedRows, InsertedRows, ValuesWouldChange int64
	LastSeq                                              int64
}

// Reprocessor refaz as linhas derivadas.
type Reprocessor struct {
	repos port.Repositories // fora de transação
	tx    port.Transactor
	cat   pid.Query
	met   port.MetricsReprocess
	log   *slog.Logger
}

// New liga o caso de uso às portas e ao catálogo de PIDs.
func New(repos port.Repositories, tx port.Transactor, cat pid.Query, met port.MetricsReprocess, log *slog.Logger) *Reprocessor {
	return &Reprocessor{repos: repos, tx: tx, cat: cat, met: met, log: log}
}

// Run refaz reading/position a partir de raw_block. Sem Apply, é só um ensaio.
func (c *Reprocessor) Run(ctx context.Context, o Options) (Summary, error) {
	var res Summary
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
	for start <= o.ToSeq {
		blocks, err := c.repos.Blocks.ListData(ctx, o.DongleID, start, o.ToSeq, o.Batch)
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
		var ls []port.ReadingAt
		var ps []port.PositionAt
		for _, x := range blocks {
			blk, err := protocol.DecodeBlock(append(append([]byte{}, x.Payload...), make([]byte, 0)...), protocol.Profile{Name: "raw"})
			if err != nil {
				return res, fmt.Errorf("seq %d: %w", x.Seq, err)
			}
			cont, err := protocol.ReadRecords(blk.Data)
			if err != nil {
				return res, fmt.Errorf("seq %d: %w", x.Seq, err)
			}
			l, p, err := pid.Decode(uint16(x.Generation), blk.Hdr, cont, x.T0Ms, c.cat)
			if err != nil {
				return res, fmt.Errorf("seq %d: %w (nada foi alterado nesta janela)", x.Seq, err)
			}
			for _, y := range l {
				ls = append(ls, port.ReadingAt{Reading: y, ReceivedAt: x.ReceivedAt})
			}
			for _, y := range p {
				ps = append(ps, port.PositionAt{Position: y, ReceivedAt: x.ReceivedAt})
			}
		}
		res.Blocks += int64(len(blocks))
		res.LastSeq = b
		if !o.Apply {
			n, err := c.repos.Blocks.CountReadings(ctx, o.DongleID, a, b)
			if err != nil {
				return res, err
			}
			res.RemovedRows += n
			res.InsertedRows += int64(len(ls))
		} else {
			var rem int64
			err = nil
			for attempt := 0; attempt < 5; attempt++ {
				err = c.tx.InTransaction(ctx, func(ctx context.Context, r port.Repositories) error {
					if err := r.Dongles.Lock(ctx, o.DongleID); err != nil {
						return err
					}
					n1, err := r.Blocks.DeleteReadings(ctx, o.DongleID, a, b, minR, maxR)
					if err != nil {
						return err
					}
					if _, err := r.Blocks.DeletePositions(ctx, o.DongleID, a, b, minR, maxR); err != nil {
						return err
					}
					if len(ls) > 0 {
						if err := r.Blocks.CopyReadingsAt(ctx, o.DongleID, ls); err != nil {
							return err
						}
					}
					if len(ps) > 0 {
						if err := r.Blocks.CopyPositionsAt(ctx, o.DongleID, ps); err != nil {
							return err
						}
					}
					rem = n1
					return r.Boots.MarkGold(ctx, o.DongleID, a, b)
				})
				if !errors.Is(err, port.ErrDatabaseBusy) {
					break
				}
			}
			if err != nil {
				return res, err
			}
			res.RemovedRows += rem
			res.InsertedRows += int64(len(ls))
			c.met.Reprocessed(len(ls))
		}
		start = b + 1
	}
	if o.Apply {
		det, _ := json.Marshal(map[string]any{"from_seq": o.FromSeq, "to_seq": o.ToSeq, "blocks": res.Blocks,
			"removed_rows": res.RemovedRows, "inserted_rows": res.InsertedRows, "applied": true, "operator": o.Operator})
		if err := c.repos.Occurrences.InsertAlert(ctx, occurrence.Alert{DongleID: o.DongleID, Type: "reprocessing", Detail: det}); err != nil {
			return res, err
		}
		c.log.Info("ingestao.reprocessado", "dongle_id", o.DongleID, "blocos", res.Blocks, "inseridas", res.InsertedRows)
	}
	return res, nil
}
