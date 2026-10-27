package banco

import (
	"context"
	"database/sql"
	"embed"
	"io/fs"

	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

//go:embed migrations/*.sql
var Migracoes embed.FS

func provider(db *sql.DB) (*goose.Provider, error) {
	sub, err := fs.Sub(Migracoes, "migrations")
	if err != nil {
		return nil, err
	}
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return nil, err
	}
	return goose.NewProvider(goose.DialectPostgres, db, sub, goose.WithSessionLocker(locker))
}

// Migrar aplica todas as migrations pendentes, sob o advisory lock de sessão do goose.
func Migrar(ctx context.Context, db *sql.DB) error {
	p, err := provider(db)
	if err != nil {
		return err
	}
	_, err = p.Up(ctx)
	return err
}

// DescerAte desfaz até a versão informada (0 = tudo).
func DescerAte(ctx context.Context, db *sql.DB, versao int64) error {
	p, err := provider(db)
	if err != nil {
		return err
	}
	_, err = p.DownTo(ctx, versao)
	return err
}

// Status devolve a situação de cada migration.
func Status(ctx context.Context, db *sql.DB) ([]*goose.MigrationStatus, error) {
	p, err := provider(db)
	if err != nil {
		return nil, err
	}
	return p.Status(ctx)
}
