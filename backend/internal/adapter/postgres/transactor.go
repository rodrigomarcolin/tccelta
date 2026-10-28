// Package postgres implementa os repositórios (port.Repositories) sobre o sqlc. Junto com database concentra pgx e as
// consultas (o worker e as ferramentas acessam database/dbgen direto, exceção registrada no depguard): a tradução de pgx.ErrNoRows para erros de domínio e das structs do sqlc
// para tipos de domínio acontece aqui.
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
		Dongles:     &dongles{q},
		Challenges:  &challenges{q},
		States:      &states{q},
		Boots:       &boots{q},
		Blocks:      &blocks{q},
		Occurrences: &occurrences{q},
		Pids:        &pids{q},
	}
}

// Transactor abre transações no pool com banco.InTransaction (timeouts, retry de deadlock).
type Transactor struct {
	pool *pgxpool.Pool
	ops  database.TxOptions
}

// NewTransactor usa banco.DefaultOptions.
func NewTransactor(pool *pgxpool.Pool) *Transactor {
	return &Transactor{pool: pool, ops: database.DefaultOptions}
}

// InTransaction implementa port.Transactor.
func (t *Transactor) InTransaction(ctx context.Context, fn func(ctx context.Context, r port.Repositories) error) error {
	return database.InTransaction(ctx, t.pool, t.ops, func(ctx context.Context, q *dbgen.Queries, _ pgx.Tx) error {
		return fn(ctx, Repositories(q))
	})
}

var _ port.Transactor = (*Transactor)(nil)
