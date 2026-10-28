package api

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"tccelta/backend/internal/port"
	"tccelta/backend/internal/usecase/ingestion"
)

// receiveBatch: POST /v1/dongles/blocos.
func (d *Deps) receiveBatch(w http.ResponseWriter, r *http.Request) {
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/octet-stream") {
		problem(w, 415, "tipo_conteudo", "esperado application/octet-stream", 0)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var mb *http.MaxBytesError
		if errors.As(err, &mb) {
			problem(w, 413, "corpo_grande", "lote acima do limite", 0)
			return
		}
		problem(w, 400, "lote_malformado", "falha ao ler o corpo", 0)
		return
	}
	s := r.Context().Value(ctxSession).(port.Session)
	res, err := d.ReceiveBatch.Run(r.Context(), ingestion.ReceiveBatchInput{Session: s, Body: body})
	if err != nil {
		d.err(w, err)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "application/octet-stream")
	if res.Interrupted {
		h.Set("X-Lote-Status", "interrompido")
		h.Set("X-Lote-Motivo", wireReason(res.Reason))
	} else {
		h.Set("X-Lote-Status", "completo")
	}
	h.Set("X-Lote-Acao", string(res.Action))
	h.Set("X-Lote-Aceitos", strconv.Itoa(res.Accepted))
	h.Set("X-Lote-Duplicados", strconv.Itoa(res.Duplicates))
	if res.NoProgress {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}
	_, _ = w.Write(res.AckTLV)
}
