package api

import (
	"net/http"

	"tccelta/backend/internal/port"
	"tccelta/backend/internal/usecase/ingestion"
)

// getConfirmation: GET /v1/dongles/{id}/confirmacao.
func (d *Deps) getConfirmation(w http.ResponseWriter, r *http.Request) {
	s := r.Context().Value(ctxSession).(port.Session)
	ack, err := d.GetConfirmation.Run(r.Context(), ingestion.GetConfirmationInput{DevID: s.Dev})
	if err != nil {
		d.err(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	_, _ = w.Write(ack)
}
