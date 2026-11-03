package visitantes

import (
	"context"
	"path/filepath"
	"time"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestIncrementaConcorrente(t *testing.T) {
	c := Novo(427)
	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() { defer wg.Done(); c.Incrementa() }()
	}
	wg.Wait()
	if got := c.Total(); got != 527 {
		t.Fatalf("total = %d, esperado 527", got)
	}
}

func TestHandler(t *testing.T) {
	h := Novo(10).Handler()
	do := func(method, path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(method, path, nil))
		return w
	}
	if b := do("GET", "/api/visitantes").Body.String(); !strings.Contains(b, `"total":10`) {
		t.Fatalf("GET: %s", b)
	}
	if b := do("POST", "/api/visitantes").Body.String(); !strings.Contains(b, `"total":11`) {
		t.Fatalf("POST: %s", b)
	}
	if b := do("GET", "/api/visitantes").Body.String(); !strings.Contains(b, `"total":11`) {
		t.Fatalf("GET apos POST: %s", b)
	}
	if c := do("GET", "/saude/viva").Code; c != http.StatusOK {
		t.Fatalf("saude: %d", c)
	}
}

func TestPersistenciaSobreviveARestart(t *testing.T) {
	arq := filepath.Join(t.TempDir(), "total")
	ctx, cancel := context.WithCancel(context.Background())
	c := Novo(14)
	c.Carrega(arq) // sem arquivo: mantem o inicial
	if c.Total() != 14 {
		t.Fatalf("inicial = %d", c.Total())
	}
	done := make(chan struct{})
	go func() { c.Persiste(ctx, arq, time.Millisecond); close(done) }()
	c.Incrementa()
	c.Incrementa()
	cancel()
	<-done

	novo := Novo(0) // o arquivo vence o valor inicial
	novo.Carrega(arq)
	if novo.Total() != 16 {
		t.Fatalf("apos restart = %d, esperado 16", novo.Total())
	}
}
