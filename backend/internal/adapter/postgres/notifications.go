package postgres

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"tccelta/backend/internal/database"
	"tccelta/backend/internal/port"
)

// Notifications implementa port.Notifications com LISTEN numa conexão dedicada do pool.
type Notifications struct {
	pool *pgxpool.Pool
	log  *slog.Logger
}

// NewNotifications escuta pelo pool; as quedas da conexão são registradas em log.
func NewNotifications(pool *pgxpool.Pool, log *slog.Logger) *Notifications {
	return &Notifications{pool: pool, log: log}
}

// Listen implementa port.Notifications.
func (n *Notifications) Listen(ctx context.Context, channel string, reconnections func()) <-chan string {
	return database.Listen(ctx, n.pool, channel, n.log, reconnections)
}

var _ port.Notifications = (*Notifications)(nil)
