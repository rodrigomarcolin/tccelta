package httpapi

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"tccelta/backend/internal/casodeuso/ingestao"
	"tccelta/backend/internal/casodeuso/porta"
)

// receberLote: POST /v1/dongles/blocos.
func (d *Deps) receberLote(w http.ResponseWriter, r *http.Request) {
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/octet-stream") {
		problema(w, 415, "tipo_conteudo", "esperado application/octet-stream", 0)
		return
	}
	corpo, err := io.ReadAll(r.Body)
	if err != nil {
		var mb *http.MaxBytesError
		if errors.As(err, &mb) {
			problema(w, 413, "corpo_grande", "lote acima do limite", 0)
			return
		}
		problema(w, 400, "lote_malformado", "falha ao ler o corpo", 0)
		return
	}
	s := r.Context().Value(ctxSessao).(porta.Sessao)
	res, err := d.ReceberLote.Executar(r.Context(), ingestao.ReceberLoteEntrada{Sessao: s, Corpo: corpo})
	if err != nil {
		d.erro(w, err)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "application/octet-stream")
	if res.Interrompido {
		h.Set("X-Lote-Status", "interrompido")
		h.Set("X-Lote-Motivo", string(res.Motivo))
	} else {
		h.Set("X-Lote-Status", "completo")
	}
	h.Set("X-Lote-Acao", string(res.Acao))
	h.Set("X-Lote-Aceitos", strconv.Itoa(res.Aceitos))
	h.Set("X-Lote-Duplicados", strconv.Itoa(res.Duplicados))
	if res.SemProgresso {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}
	_, _ = w.Write(res.AckTLV)
}
