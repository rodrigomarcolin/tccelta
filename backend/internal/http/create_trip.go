package http

import (
	"cmp"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"

	"tccelta/backend/internal/domain/trip"
)

// maxTripBody é o limite do corpo do POST /v1/viagens (4 KiB).
const maxTripBody = 4096

type createTripBody struct {
	Driver struct {
		Name *string `json:"nome"`
	} `json:"motorista"`
	Category *string `json:"categoria_viagem"`
	Fuel     *string `json:"combustivel"`
	Entry    struct {
		Type           *string `json:"tipo"`
		DongleID       *int64  `json:"dongle_id"`
		BootID         *int64  `json:"boot_id"`
		BootGeneration *int64  `json:"boot_generation"`
		T0             *string `json:"t0"`
		T1             *string `json:"t1"`
	} `json:"entrada"`
}

type createTripOutput struct {
	ID     string `json:"viagem_id"`
	Status string `json:"status"`
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// createTrip: POST /v1/viagens.
func (d *Deps) createTrip(w http.ResponseWriter, r *http.Request) {
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
		problem(w, 415, "tipo_conteudo", "esperado application/json", 0)
		return
	}
	var b createTripBody
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	err := dec.Decode(&b)
	if err == nil { // depois do objeto não pode haver mais nada
		if _, e := dec.Token(); !errors.Is(e, io.EOF) {
			err = cmp.Or(e, errors.New("dados depois do objeto JSON"))
		}
	}
	if err != nil {
		var mb *http.MaxBytesError
		if errors.As(err, &mb) {
			problem(w, 413, "corpo_grande", "corpo acima do limite", 0)
			return
		}
		problem(w, 400, "corpo_malformado", "JSON inválido, de tipo errado ou com campo desconhecido", 0)
		return
	}
	t, err := d.CreateTrip.Run(r.Context(), trip.Input{
		DriverName: str(b.Driver.Name), Category: str(b.Category), Fuel: str(b.Fuel), EntryType: str(b.Entry.Type),
		DongleID: b.Entry.DongleID, BootID: b.Entry.BootID, BootGeneration: b.Entry.BootGeneration, T0: b.Entry.T0, T1: b.Entry.T1})
	if err != nil {
		d.err(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Location", "/v1/viagens/"+t.ID.String())
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(createTripOutput{ID: t.ID.String(), Status: tripStatus(t.Status)})
}
