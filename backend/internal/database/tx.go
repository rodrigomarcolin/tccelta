package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"tccelta/backend/internal/database/dbgen"
	"tccelta/backend/internal/repository"
)

// ErrDatabaseBusy: lock_timeout, deadlock ou falha de serialização persistente (-> 503).
var ErrDatabaseBusy = repository.ErrDatabaseBusy

// TxOptions são os timeouts locais da transação.
type TxOptions struct{ LockTimeout, StatementTimeout time.Duration }

// DefaultOptions: lock_timeout 3 s, statement_timeout 10 s.
var DefaultOptions = TxOptions{LockTimeout: 3 * time.Second, StatementTimeout: 10 * time.Second}

func code(err error) string {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		return pe.Code
	}
	return ""
}

// InTransaction abre uma transação READ COMMITTED, aplica SET LOCAL dos timeouts, executa fn e faz
// commit/rollback. 55P03 vira ErrDatabaseBusy; 40P01 e 40001 repetem fn uma vez.
func InTransaction(ctx context.Context, pool *pgxpool.Pool, o TxOptions,
	fn func(ctx context.Context, q *dbgen.Queries, tx pgx.Tx) error) error {
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		err = oneTransaction(ctx, pool, o, fn)
		switch code(err) {
		case "40P01", "40001":
			continue
		}
		break
	}
	switch code(err) {
	case "55P03", "40P01", "40001", "57014":
		return fmt.Errorf("%w: %v", ErrDatabaseBusy, err)
	}
	return err
}

func oneTransaction(ctx context.Context, pool *pgxpool.Pool, o TxOptions,
	fn func(ctx context.Context, q *dbgen.Queries, tx pgx.Tx) error) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
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
	if err := fn(ctx, dbgen.New(tx), tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
