package http

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"tccelta/backend/internal/usecase/handshake"
)

type responseBody struct {
	Response string `json:"resposta"`
}

type handshakeOutput struct {
	Ack            string    `json:"ack"`
	Token          string    `json:"token"`
	TokenExpiresAt time.Time `json:"token_expira_em"`
	TokenValidityS int       `json:"token_validade_s"`
	TLatS          *int      `json:"t_lat_s,omitempty"`
}

// respondChallenge: POST /v1/dongles/{id}/desafio/resposta.
func (d *Deps) respondChallenge(w http.ResponseWriter, r *http.Request) {
	s2, _ := r.Context().Value(ctxS2).(time.Time)
	if s2.IsZero() {
		s2 = time.Now()
	}
	dev, ok := devFromPath(w, r)
	if !ok {
		return
	}
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		problem(w, 415, "tipo_conteudo", "esperado application/json", 0)
		return
	}
	var c responseBody
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		var mb *http.MaxBytesError
		if errors.As(err, &mb) {
			problem(w, 413, "corpo_grande", "corpo acima de 2 KiB", 0)
			return
		}
		problem(w, 400, "corpo_malformado", "JSON inválido", 0)
		return
	}
	tlv, err := base64.StdEncoding.Strict().DecodeString(c.Response)
	if err != nil {
		problem(w, 400, "base64_invalido", "base64 padrão com padding esperado", 0)
		return
	}
	res, err := d.RespondChallenge.Run(r.Context(), handshake.RespondChallengeInput{DevID: dev, S2: s2, TLV: tlv})
	if err != nil {
		d.err(w, err)
		return
	}
	out := handshakeOutput{Ack: base64.StdEncoding.EncodeToString(res.AckTLV), Token: res.Token,
		TokenExpiresAt: res.TokenExpiresAt.UTC(), TokenValidityS: res.ValidityS}
	if res.SuggestedTLat > 0 {
		t := int(res.SuggestedTLat.Seconds())
		out.TLatS = &t
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(out)
}
