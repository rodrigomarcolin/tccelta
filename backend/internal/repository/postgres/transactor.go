// Package postgres implementa os repositórios sobre o sqlc (consultas). É o único lugar da API que
// importa consultas e pgx: a tradução de pgx.ErrNoRows para erros de domínio e das structs do sqlc
// para tipos de domínio acontece aqui.
package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"tccelta/backend/internal/database"
	"tccelta/backend/internal/database/dbgen"
	"tccelta/backend/internal/repository"
)

// Repositories monta os repositórios sobre q (pool ou transação).
func Repositories(q dbgen.Querier) repository.Repositories {
	return repository.Repositories{
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

// InTransaction implementa repository.Transactor.
func (t *Transactor) InTransaction(ctx context.Context, fn func(ctx context.Context, r repository.Repositories) error) error {
	return database.InTransaction(ctx, t.pool, t.ops, func(ctx context.Context, q *dbgen.Queries, _ pgx.Tx) error {
		return fn(ctx, Repositories(q))
	})
}

var _ repository.Transactor = (*Transactor)(nil)
