package httpapi

import (
	"net/http"
	"time"
)

func (d *Deps) viva(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }

func (d *Deps) pronta(w http.ResponseWriter, r *http.Request) {
	if (d.NaoPronto != nil && d.NaoPronto.Load()) || (d.Prontidao != nil && !d.Prontidao()) {
		problema(w, 503, "nao_pronto", "dependências indisponíveis", time.Second)
		return
	}
	w.WriteHeader(http.StatusOK)
}
