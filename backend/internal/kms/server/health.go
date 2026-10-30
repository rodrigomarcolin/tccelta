package server

import (
	"encoding/json"
	"net/http"
	"time"
)

func (s *Server) live(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }

// ready: token do Vault válido e disjuntor fechado.
func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	if !s.Vault.Ready() {
		problem(w, 503, "nao_pronto", "Vault indisponível", time.Second)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}

// HealthRoutes registra /saude/viva e /saude/pronta em mux. O servidor principal as expõe (atrás do
// mTLS) e a porta de métricas as repete em HTTP simples, para o healthcheck do contêiner.
func (s *Server) HealthRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /saude/viva", s.live)
	mux.HandleFunc("GET /saude/pronta", s.ready)
}
