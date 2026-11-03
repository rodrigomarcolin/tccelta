package visitantes

import (
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
