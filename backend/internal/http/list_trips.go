package http

import (
	"encoding/json"
	"net/http"
	"strconv"

	"tccelta/backend/internal/domain/trip"
)

type tripListItemOutput struct {
	ID          string `json:"viagem_id"`
	Status      string `json:"status"`
	Provisional bool   `json:"provisoria"`
	Driver      struct {
		Name string `json:"nome"`
	} `json:"motorista"`
	Category       string         `json:"categoria_viagem"`
	EntryType      string         `json:"entrada_tipo"`
	DongleID       int32          `json:"dongle_id"`
	BootID         *int32         `json:"boot_id"`
	BootGeneration *int32         `json:"boot_generation"`
	StartedAt      *string        `json:"inicio_em"`
	EndedAt        *string        `json:"fim_em"`
	DistanceKm     *float64       `json:"distancia_km"`
	DurationS      *float64       `json:"duracao_s"`
	CreatedAt      string         `json:"criada_em"`
	RoutePreview   [][]trip.Point `json:"rota_resumo"`
}

type tripListOutput struct {
	Trips      []tripListItemOutput `json:"viagens"`
	Total      int64                `json:"total"`
	NextCursor *string              `json:"proximo_cursor"`
}

// listQueryFrom lê limite, cursor e busca. Parâmetro presente e vazio vale como ausente, menos limite, que precisa ser um inteiro.
func listQueryFrom(r *http.Request) (trip.ListQuery, error) {
	v := r.URL.Query()
	var limit *int64
	if v.Has("limite") {
		n, err := strconv.ParseInt(v.Get("limite"), 10, 64)
		if err != nil {
			return trip.ListQuery{}, trip.FieldError{Field: "limite", Reason: "deve ser um inteiro"}
		}
		limit = &n
	}
	return trip.NewListQuery(limit, v.Get("cursor"), v.Get("busca"))
}

// listTrips: GET /v1/viagens.
func (d *Deps) listTrips(w http.ResponseWriter, r *http.Request) {
	q, err := listQueryFrom(r)
	if err != nil {
		d.err(w, err)
		return
	}
	page, err := d.ListTrips.Run(r.Context(), q)
	if err != nil {
		d.err(w, err)
		return
	}
	out := tripListOutput{Trips: make([]tripListItemOutput, len(page.Items)), Total: page.Total}
	for i, it := range page.Items {
		start, end := it.Period()
		o := tripListItemOutput{ID: it.ID.String(), Status: tripStatus(it.Status), Provisional: it.Provisional, Category: it.Category,
			EntryType: wireEntryType(it.EntryType), DongleID: it.DongleID, StartedAt: utcMsPtr(start), EndedAt: utcMsPtr(end),
			DistanceKm: it.DistanceKm, DurationS: it.DurationS, CreatedAt: utcMs(it.CreatedAt), RoutePreview: it.Route}
		o.Driver.Name = it.DriverName
		if it.EntryType == trip.EntryTypeBoot { // a entrada por intervalo pode cobrir vários boots
			o.BootID, o.BootGeneration = it.BootID, it.BootGeneration
		}
		out.Trips[i] = o
	}
	if page.Next != nil {
		c := page.Next.Encode()
		out.NextCursor = &c
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(out)
}

// wireEntryType traduz entry_type (boot | interval) para o nome da API (boot | intervalo).
func wireEntryType(s string) string {
	if s == trip.EntryTypeInterval {
		return "intervalo"
	}
	return "boot"
}
