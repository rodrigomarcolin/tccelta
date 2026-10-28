package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	dhs "tccelta/backend/internal/domain/handshake"
	"tccelta/backend/internal/domain/protocol"
	"tccelta/backend/internal/usecase/handshake"
)

type challengeOutput struct {
	Challenge string    `json:"desafio"`
	ExpiresAt time.Time `json:"expira_em"`
}

// devFromPath lê {id} (DevId hex); responde 400 se for inválido.
func devFromPath(w http.ResponseWriter, r *http.Request) (protocol.DevID, bool) {
	dev, err := protocol.DevIDFromHex(r.PathValue("id"))
	if err != nil {
		problem(w, 400, "devid_invalido", "DevId deve ter 16 dígitos hexadecimais", 0)
		return protocol.DevID{}, false
	}
	return dev, true
}

// requestChallenge: POST /v1/dongles/{id}/desafio.
func (d *Deps) requestChallenge(w http.ResponseWriter, r *http.Request) {
	dev, ok := devFromPath(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 0)
	out, err := d.RequestChallenge.Run(r.Context(), handshake.IssueChallengeInput{DevID: dev})
	if err != nil {
		if errors.Is(err, dhs.ErrPendingChallenges) {
			problem(w, 429, "desafios_pendentes", "há desafios pendentes demais", out.RetryAfter)
			return
		}
		d.err(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(challengeOutput{Challenge: base64.StdEncoding.EncodeToString(out.TLV), ExpiresAt: out.ExpiresAt.UTC()})
}
