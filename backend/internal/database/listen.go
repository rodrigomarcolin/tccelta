package database

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Listen mantém uma conexão dedicada com LISTEN <canal> e entrega as cargas no canal devolvido.
// Ao reconectar emite uma carga vazia sintética (notificações podem ter sido perdidas).
// reconexoes (opcional) é chamada a cada reconexão.
func Listen(ctx context.Context, pool *pgxpool.Pool, channel string, log *slog.Logger, reconnections func()) <-chan string {
	out := make(chan string, 64)
	go func() {
		defer close(out)
		wait := time.Second
		first := true
		for ctx.Err() == nil {
			err := listenOnce(ctx, pool, channel, out, &first, reconnections)
			if ctx.Err() != nil {
				return
			}
			log.Warn("banco.escuta_caiu", "canal", channel, "erro", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
			if wait < 30*time.Second {
				wait *= 2
			}
		}
	}()
	return out
}

func listenOnce(ctx context.Context, pool *pgxpool.Pool, channel string, out chan<- string, first *bool, reconnections func()) error {
	c, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer c.Release()
	conn := c.Conn()
	if _, err := conn.Exec(ctx, "LISTEN "+pgx.Identifier{channel}.Sanitize()); err != nil {
		return err
	}
	if !*first {
		if reconnections != nil {
			reconnections()
		}
		send(out, "")
	}
	*first = false
	for {
		n, err := conn.WaitForNotification(ctx)
		if err != nil {
			// a conexão pode estar quebrada: descarta do pool
			_ = conn.Close(context.WithoutCancel(ctx))
			return err
		}
		send(out, n.Payload)
	}
}

func send(out chan<- string, p string) {
	select {
	case out <- p:
	default: // canal cheio: o consumidor coalesce de qualquer forma
	}
}
