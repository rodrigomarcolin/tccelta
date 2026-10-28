package httpapi

import (
	"net/http"

	"tccelta/backend/internal/casodeuso/ingestao"
	"tccelta/backend/internal/casodeuso/porta"
)

// obterConfirmacao: GET /v1/dongles/{id}/confirmacao.
func (d *Deps) obterConfirmacao(w http.ResponseWriter, r *http.Request) {
	s := r.Context().Value(ctxSessao).(porta.Sessao)
	ack, err := d.ObterConfirmacao.Executar(r.Context(), ingestao.ObterConfirmacaoEntrada{DevID: s.Dev})
	if err != nil {
		d.erro(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	_, _ = w.Write(ack)
}
