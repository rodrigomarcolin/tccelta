package api

import (
	"net/http"
	"time"
)

func (d *Deps) live(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }

func (d *Deps) ready(w http.ResponseWriter, r *http.Request) {
	if (d.NotReady != nil && d.NotReady.Load()) || (d.Readiness != nil && !d.Readiness()) {
		problem(w, 503, "nao_pronto", "dependências indisponíveis", time.Second)
		return
	}
	w.WriteHeader(http.StatusOK)
}
