package banco

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"tccelta/backend/internal/banco/consultas"
	"tccelta/backend/internal/repositorio"
)

// ErrBancoOcupado: lock_timeout, deadlock ou falha de serialização persistente (-> 503).
var ErrBancoOcupado = repositorio.ErrBancoOcupado

// OpcoesTx são os timeouts locais da transação.
type OpcoesTx struct{ LockTimeout, StatementTimeout time.Duration }

// OpcoesPadrao: lock_timeout 3 s, statement_timeout 10 s.
var OpcoesPadrao = OpcoesTx{LockTimeout: 3 * time.Second, StatementTimeout: 10 * time.Second}

func codigo(err error) string {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		return pe.Code
	}
	return ""
}

// EmTransacao abre uma transação READ COMMITTED, aplica SET LOCAL dos timeouts, executa fn e faz
// commit/rollback. 55P03 vira ErrBancoOcupado; 40P01 e 40001 repetem fn uma vez.
func EmTransacao(ctx context.Context, pool *pgxpool.Pool, o OpcoesTx,
	fn func(ctx context.Context, q *consultas.Queries, tx pgx.Tx) error) error {
	var err error
	for tentativa := 0; tentativa < 2; tentativa++ {
		err = umaTransacao(ctx, pool, o, fn)
		switch codigo(err) {
		case "40P01", "40001":
			continue
		}
		break
	}
	switch codigo(err) {
	case "55P03", "40P01", "40001", "57014":
		return fmt.Errorf("%w: %v", ErrBancoOcupado, err)
	}
	return err
}

func umaTransacao(ctx context.Context, pool *pgxpool.Pool, o OpcoesTx,
	fn func(ctx context.Context, q *consultas.Queries, tx pgx.Tx) error) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if o.LockTimeout > 0 {
		if _, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL lock_timeout = '%dms'", o.LockTimeout.Milliseconds())); err != nil {
			return err
		}
	}
	if o.StatementTimeout > 0 {
		if _, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL statement_timeout = '%dms'", o.StatementTimeout.Milliseconds())); err != nil {
			return err
		}
	}
	if err := fn(ctx, consultas.New(tx), tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
