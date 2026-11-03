// Package visitantes e o contador de visitantes do site estatico. O valor vive so em memoria (zera ao reiniciar)
// e por isso o servico roda em uma unica instancia, separado das replicas da API.
package visitantes

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
)

// Contador e seguro para uso concorrente.
type Contador struct{ n atomic.Uint64 }

// Novo cria um contador comecando em inicial.
func Novo(inicial uint64) *Contador {
	c := &Contador{}
	c.n.Store(inicial)
	return c
}

// Total devolve o valor atual.
func (c *Contador) Total() uint64 { return c.n.Load() }

// Incrementa soma um visitante e devolve o novo total.
func (c *Contador) Incrementa() uint64 { return c.n.Add(1) }

// Handler serve GET/POST /api/visitantes e GET /saude/viva.
func (c *Contador) Handler() http.Handler {
	mux := http.NewServeMux()
	resp := func(w http.ResponseWriter, total uint64) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]uint64{"total": total})
	}
	mux.HandleFunc("GET /api/visitantes", func(w http.ResponseWriter, _ *http.Request) { resp(w, c.Total()) })
	mux.HandleFunc("POST /api/visitantes", func(w http.ResponseWriter, _ *http.Request) { resp(w, c.Incrementa()) })
	mux.HandleFunc("GET /saude/viva", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	return mux
}
