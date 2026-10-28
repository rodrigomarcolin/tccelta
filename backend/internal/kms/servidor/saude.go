package servidor

import (
	"encoding/json"
	"net/http"
	"time"
)

func (s *Servidor) viva(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }

// pronta: token do Vault válido e disjuntor fechado.
func (s *Servidor) pronta(w http.ResponseWriter, r *http.Request) {
	if !s.Vault.Pronto() {
		problema(w, 503, "nao_pronto", "Vault indisponível", time.Second)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func escrever(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}

// RotasSaude registra /saude/viva e /saude/pronta em mux. O servidor principal as expõe (atrás do
// mTLS) e a porta de métricas as repete em HTTP simples, para o healthcheck do contêiner.
func (s *Servidor) RotasSaude(mux *http.ServeMux) {
	mux.HandleFunc("GET /saude/viva", s.viva)
	mux.HandleFunc("GET /saude/pronta", s.pronta)
}
