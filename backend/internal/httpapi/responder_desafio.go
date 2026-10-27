package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"tccelta/backend/internal/casodeuso/handshake"
)

type corpoResposta struct {
	Resposta string `json:"resposta"`
}

type saidaHandshake struct {
	Ack            string    `json:"ack"`
	Token          string    `json:"token"`
	TokenExpiraEm  time.Time `json:"token_expira_em"`
	TokenValidadeS int       `json:"token_validade_s"`
	TLatS          *int      `json:"t_lat_s,omitempty"`
}

// responderDesafio: POST /v1/dongles/{id}/desafio/resposta.
func (d *Deps) responderDesafio(w http.ResponseWriter, r *http.Request) {
	s2, _ := r.Context().Value(ctxS2).(time.Time)
	if s2.IsZero() {
		s2 = time.Now()
	}
	dev, ok := devDoCaminho(w, r)
	if !ok {
		return
	}
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		problema(w, 415, "tipo_conteudo", "esperado application/json", 0)
		return
	}
	var c corpoResposta
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		var mb *http.MaxBytesError
		if errors.As(err, &mb) {
			problema(w, 413, "corpo_grande", "corpo acima de 2 KiB", 0)
			return
		}
		problema(w, 400, "corpo_malformado", "JSON inválido", 0)
		return
	}
	tlv, err := base64.StdEncoding.Strict().DecodeString(c.Resposta)
	if err != nil {
		problema(w, 400, "base64_invalido", "base64 padrão com padding esperado", 0)
		return
	}
	res, err := d.ResponderDesafio.Executar(r.Context(), handshake.ResponderDesafioEntrada{DevID: dev, S2: s2, TLV: tlv})
	if err != nil {
		d.erro(w, err)
		return
	}
	out := saidaHandshake{Ack: base64.StdEncoding.EncodeToString(res.AckTLV), Token: res.Token,
		TokenExpiraEm: res.TokenExpiraEm.UTC(), TokenValidadeS: res.ValidadeS}
	if res.TLatSugerido > 0 {
		t := int(res.TLatSugerido.Seconds())
		out.TLatS = &t
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(out)
}
