package banco

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Escutar mantém uma conexão dedicada com LISTEN <canal> e entrega as cargas no canal devolvido.
// Ao reconectar emite uma carga vazia sintética (notificações podem ter sido perdidas).
// reconexoes (opcional) é chamada a cada reconexão.
func Escutar(ctx context.Context, pool *pgxpool.Pool, canal string, log *slog.Logger, reconexoes func()) <-chan string {
	out := make(chan string, 64)
	go func() {
		defer close(out)
		espera := time.Second
		primeira := true
		for ctx.Err() == nil {
			err := escutarUmaVez(ctx, pool, canal, out, &primeira, reconexoes)
			if ctx.Err() != nil {
				return
			}
			log.Warn("banco.escuta_caiu", "canal", canal, "erro", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(espera):
			}
			if espera < 30*time.Second {
				espera *= 2
			}
		}
	}()
	return out
}

func escutarUmaVez(ctx context.Context, pool *pgxpool.Pool, canal string, out chan<- string, primeira *bool, reconexoes func()) error {
	c, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer c.Release()
	conn := c.Conn()
	if _, err := conn.Exec(ctx, "LISTEN "+pgx.Identifier{canal}.Sanitize()); err != nil {
		return err
	}
	if !*primeira {
		if reconexoes != nil {
			reconexoes()
		}
		enviar(out, "")
	}
	*primeira = false
	for {
		n, err := conn.WaitForNotification(ctx)
		if err != nil {
			// a conexão pode estar quebrada: descarta do pool
			_ = conn.Close(context.WithoutCancel(ctx))
			return err
		}
		enviar(out, n.Payload)
	}
}

func enviar(out chan<- string, p string) {
	select {
	case out <- p:
	default: // canal cheio: o consumidor coalesce de qualquer forma
	}
}
