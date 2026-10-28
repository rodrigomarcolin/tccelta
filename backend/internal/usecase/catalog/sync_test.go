package catalog

import (
	"context"
	"testing"
	"time"

	"tccelta/backend/internal/adapter/memory"
	"tccelta/backend/internal/domain/pid"
)

func TestReloadAndTrack(t *testing.T) {
	db := memory.New()
	cat := pid.New()
	s := NewSync(db.Repositories().Pids, cat, nil)
	if cat.Loaded() {
		t.Fatal("começa vazio")
	}
	db.Defs = []pid.Def{{PID: 12, NBytes: 2, Factor: 0.25}}
	if err := s.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	if d, ok := cat.Get(12); !ok || d.NBytes != 2 || !cat.Loaded() {
		t.Fatalf("%+v %v", d, ok)
	}

	// um sinal de LISTEN pid_changed dispara a recarga
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	signals := make(chan string)
	go s.Track(ctx, signals)
	db.Defs = []pid.Def{{PID: 12, NBytes: 2, Factor: 0.25}, {PID: 5, NBytes: 1, Factor: 1}}
	signals <- "pid_changed"
	for i := 0; i < 100; i++ {
		if _, ok := cat.Get(5); ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("catálogo não recarregou depois do sinal")
}
