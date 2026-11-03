package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"tccelta/backend/internal/visitantes"
)

// cmdVisitantes sobe o contador de visitantes do site (em memoria, instancia unica).
// VISITANTES_ENDERECO (padrao :8082) e VISITANTES_INICIAL (padrao 0, usado so
// quando nao ha arquivo) e VISITANTES_ARQUIVO (opcional: onde gravar o total para sobreviver a restarts).
func cmdVisitantes(ctx context.Context) error {
	addr := os.Getenv("VISITANTES_ENDERECO")
	if addr == "" {
		addr = ":8082"
	}
	var inicial uint64
	if v := os.Getenv("VISITANTES_INICIAL"); v != "" {
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			return fmt.Errorf("VISITANTES_INICIAL invalida: %w", err)
		}
		inicial = n
	}
	c := visitantes.Novo(inicial)
	persistido := make(chan struct{})
	if arq := os.Getenv("VISITANTES_ARQUIVO"); arq != "" {
		c.Carrega(arq) // se o arquivo existe, ele vence VISITANTES_INICIAL
		go func() { c.Persiste(ctx, arq, time.Second); close(persistido) }()
	} else {
		close(persistido)
	}
	srv := &http.Server{Addr: addr, Handler: c.Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	<-persistido // espera a gravacao final
	return nil
}
