// Package database reúne a infraestrutura de acesso ao PostgreSQL (pool, transações, LISTEN, migrations).
package database

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"tccelta/backend/internal/config"
)

// DSN injeta a senha (lida do arquivo) na URL do banco.
func DSN(cfg config.DatabaseConfig) (string, error) {
	u, err := url.Parse(cfg.URL)
	if err != nil {
		return "", fmt.Errorf("BANCO_URL inválida: %w", err)
	}
	if cfg.Password != "" && u.User != nil {
		u.User = url.UserPassword(u.User.Username(), cfg.Password)
	}
	return u.String(), nil
}

// NewPool cria o pool com o tipo geography registrado (formato binário = EWKB, tratado como bytes).
func NewPool(ctx context.Context, cfg config.DatabaseConfig, log *slog.Logger) (*pgxpool.Pool, error) {
	dsn, err := DSN(cfg)
	if err != nil {
		return nil, err
	}
	pc, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	if cfg.PoolMax > 0 {
		pc.MaxConns = cfg.PoolMax
	}
	pc.MinConns = cfg.PoolMin
	pc.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		var oid uint32
		err := c.QueryRow(ctx, "SELECT oid FROM pg_type WHERE typname = 'geography' LIMIT 1").Scan(&oid)
		if err != nil {
			if err == pgx.ErrNoRows {
				return nil // PostGIS ausente (não deve ocorrer em produção)
			}
			return err
		}
		c.TypeMap().RegisterType(&pgtype.Type{Name: "geography", OID: oid, Codec: pgtype.ByteaCodec{}})
		return nil
	}
	pool, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	log.Info("banco.conectado", "max", pc.MaxConns)
	return pool, nil
}
