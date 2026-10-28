// Package postgres implementa os repositórios (port.Repositories), o transactor, a escuta de notificações e o Inspector sobre
// o sqlc. Junto com database concentra pgx e as consultas: a tradução de pgx.ErrNoRows para erros de domínio e das structs do
// sqlc para tipos de domínio acontece aqui.
package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"tccelta/backend/internal/database"
	"tccelta/backend/internal/database/dbgen"
	"tccelta/backend/internal/port"
)

// Repositories monta os repositórios sobre q (pool ou transação).
func Repositories(q dbgen.Querier) port.Repositories {
	return port.Repositories{
		Dongles:      &dongles{q},
		Challenges:   &challenges{q},
		States:       &states{q},
		Boots:        &boots{q},
		Blocks:       &blocks{q},
		Occurrences:  &occurrences{q},
		AnchorGroups: &anchorGroups{q},
		Pids:         &pids{q},
	}
}

// Transactor abre transações no pool com banco.InTransaction (timeouts, retry de deadlock).
type Transactor struct {
	pool *pgxpool.Pool
	ops  database.TxOptions
}

// NewTransactor usa database.DefaultOptions (lock_timeout 3 s, statement_timeout 10 s).
func NewTransactor(pool *pgxpool.Pool) *Transactor {
	return NewTransactorWith(pool, database.DefaultOptions)
}

// NewTransactorWith abre as transações com timeouts próprios, para os fluxos que não usam o padrão.
func NewTransactorWith(pool *pgxpool.Pool, ops database.TxOptions) *Transactor {
	return &Transactor{pool: pool, ops: ops}
}

// InTransaction implementa port.Transactor.
func (t *Transactor) InTransaction(ctx context.Context, fn func(ctx context.Context, r port.Repositories) error) error {
	return database.InTransaction(ctx, t.pool, t.ops, func(ctx context.Context, q *dbgen.Queries, _ pgx.Tx) error {
		return fn(ctx, Repositories(q))
	})
}

var _ port.Transactor = (*Transactor)(nil)
