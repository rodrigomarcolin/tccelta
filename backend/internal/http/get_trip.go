package http

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"

	"tccelta/backend/internal/domain/trip"
)

// tripIDFromPath lê {id}. Só o UUID canônico (36 caracteres) vale; qualquer outra coisa é uma viagem desconhecida.
func tripIDFromPath(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	s := r.PathValue("id")
	id, err := uuid.Parse(s)
	if err != nil || len(s) != 36 {
		problem(w, 404, "viagem_desconhecida", "viagem desconhecida", 0)
		return uuid.UUID{}, false
	}
	return id, true
}

// utcMs formata um instante em RFC 3339 UTC com milissegundos.
func utcMs(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000Z") }

func utcMsPtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := utcMs(*t)
	return &s
}

type tripStatusOutput struct {
	ID          string `json:"viagem_id"`
	Status      string `json:"status"`
	Provisional bool   `json:"provisoria"`
	Driver      struct {
		Name string `json:"nome"`
	} `json:"motorista"`
	Category     string  `json:"categoria_viagem"`
	Fuel         string  `json:"combustivel"`
	Entry        any     `json:"entrada"`
	CreatedAt    string  `json:"criada_em"`
	CalculatedAt *string `json:"calculada_em"`
}

type bootEntryJSON struct {
	Type           string `json:"tipo"`
	DongleID       int32  `json:"dongle_id"`
	BootID         *int32 `json:"boot_id"`
	BootGeneration *int32 `json:"boot_generation"`
}

type intervalEntryJSON struct {
	Type     string `json:"tipo"`
	DongleID int32  `json:"dongle_id"`
	T0       string `json:"t0"`
	T1       string `json:"t1"`
}

// entryJSON é a entrada no formato da API: boot (com boot_generation nula até o worker resolvê-la) ou intervalo (UTC).
func entryJSON(t trip.Trip) any {
	if t.EntryType == trip.EntryTypeInterval && t.IntervalStart != nil && t.IntervalEnd != nil {
		return intervalEntryJSON{Type: "intervalo", DongleID: t.DongleID, T0: utcMs(*t.IntervalStart), T1: utcMs(*t.IntervalEnd)}
	}
	return bootEntryJSON{Type: "boot", DongleID: t.DongleID, BootID: t.BootID, BootGeneration: t.BootGeneration}
}

// getTrip: GET /v1/viagens/{id}.
func (d *Deps) getTrip(w http.ResponseWriter, r *http.Request) {
	id, ok := tripIDFromPath(w, r)
	if !ok {
		return
	}
	t, err := d.GetTrip.Run(r.Context(), id)
	if err != nil {
		d.err(w, err)
		return
	}
	out := tripStatusOutput{ID: t.ID.String(), Status: tripStatus(t.Status), Provisional: t.Provisional, Category: t.Category,
		Fuel: t.Fuel, Entry: entryJSON(t), CreatedAt: utcMs(t.CreatedAt), CalculatedAt: utcMsPtr(t.CalculatedAt)}
	out.Driver.Name = t.DriverName
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(out)
}
