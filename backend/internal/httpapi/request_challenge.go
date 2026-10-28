package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"tccelta/backend/internal/casodeuso/handshake"
	dhs "tccelta/backend/internal/dominio/handshake"
	"tccelta/backend/internal/dominio/protocolo"
)

type saidaDesafio struct {
	Desafio  string    `json:"desafio"`
	ExpiraEm time.Time `json:"expira_em"`
}

// devDoCaminho lê {id} (DevId hex); responde 400 se for inválido.
func devDoCaminho(w http.ResponseWriter, r *http.Request) (protocolo.DevID, bool) {
	dev, err := protocolo.DevIDDeHex(r.PathValue("id"))
	if err != nil {
		problema(w, 400, "devid_invalido", "DevId deve ter 16 dígitos hexadecimais", 0)
		return protocolo.DevID{}, false
	}
	return dev, true
}

// pedirDesafio: POST /v1/dongles/{id}/desafio.
func (d *Deps) pedirDesafio(w http.ResponseWriter, r *http.Request) {
	dev, ok := devDoCaminho(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 0)
	out, err := d.PedirDesafio.Executar(r.Context(), handshake.EmitirDesafioEntrada{DevID: dev})
	if err != nil {
		if errors.Is(err, dhs.ErrDesafiosPendentes) {
			problema(w, 429, "desafios_pendentes", "há desafios pendentes demais", out.RetryAfter)
			return
		}
		d.erro(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(saidaDesafio{Desafio: base64.StdEncoding.EncodeToString(out.TLV), ExpiraEm: out.ExpiraEm.UTC()})
}
