// Package postgres implementa os repositórios sobre o sqlc (consultas). É o único lugar da API que
// importa consultas e pgx: a tradução de pgx.ErrNoRows para erros de domínio e das structs do sqlc
// para tipos de domínio acontece aqui.
package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"tccelta/backend/internal/banco"
	"tccelta/backend/internal/banco/consultas"
	"tccelta/backend/internal/repositorio"
)

// Repositorios monta os repositórios sobre q (pool ou transação).
func Repositorios(q consultas.Querier) repositorio.Repositorios {
	return repositorio.Repositorios{
		Dongles:     &dongles{q},
		Desafios:    &desafios{q},
		Estados:     &estados{q},
		Boots:       &boots{q},
		Blocos:      &blocos{q},
		Ocorrencias: &ocorrencias{q},
		Pids:        &pids{q},
	}
}

// Transacionador abre transações no pool com banco.EmTransacao (timeouts, retry de deadlock).
type Transacionador struct {
	pool *pgxpool.Pool
	ops  banco.OpcoesTx
}

// NovoTransacionador usa banco.OpcoesPadrao.
func NovoTransacionador(pool *pgxpool.Pool) *Transacionador {
	return &Transacionador{pool: pool, ops: banco.OpcoesPadrao}
}

// EmTransacao implementa repositorio.Transacionador.
func (t *Transacionador) EmTransacao(ctx context.Context, fn func(ctx context.Context, r repositorio.Repositorios) error) error {
	return banco.EmTransacao(ctx, t.pool, t.ops, func(ctx context.Context, q *consultas.Queries, _ pgx.Tx) error {
		return fn(ctx, Repositorios(q))
	})
}

var _ repositorio.Transacionador = (*Transacionador)(nil)
