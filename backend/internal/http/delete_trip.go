package http

import "net/http"

// deleteTrip: DELETE /v1/viagens/{id}. Exclusão lógica; 204 sem corpo (um corpo enviado é ignorado).
func (d *Deps) deleteTrip(w http.ResponseWriter, r *http.Request) {
	id, ok := tripIDFromPath(w, r)
	if !ok {
		return
	}
	if err := d.DeleteTrip.Run(r.Context(), id); err != nil {
		d.err(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}
