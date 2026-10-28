package catalogo

import (
	"context"
	"testing"
	"time"

	"tccelta/backend/internal/dominio/pid"
	"tccelta/backend/internal/repositorio/memoria"
)

func TestRecarregarEAcompanhar(t *testing.T) {
	bd := memoria.Novo()
	cat := pid.Novo()
	s := NovoSincronizar(bd.Repositorios().Pids, cat, nil)
	if cat.Carregado() {
		t.Fatal("começa vazio")
	}
	bd.Defs = []pid.Def{{PID: 12, NBytes: 2, Fator: 0.25}}
	if err := s.Recarregar(context.Background()); err != nil {
		t.Fatal(err)
	}
	if d, ok := cat.Obter(12); !ok || d.NBytes != 2 || !cat.Carregado() {
		t.Fatalf("%+v %v", d, ok)
	}

	// um sinal de LISTEN pid_alterado dispara a recarga
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sinais := make(chan string)
	go s.Acompanhar(ctx, sinais)
	bd.Defs = []pid.Def{{PID: 12, NBytes: 2, Fator: 0.25}, {PID: 5, NBytes: 1, Fator: 1}}
	sinais <- "pid_alterado"
	for i := 0; i < 100; i++ {
		if _, ok := cat.Obter(5); ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("catálogo não recarregou depois do sinal")
}
