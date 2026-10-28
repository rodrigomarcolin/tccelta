package server

import (
	"net/http"
	"strconv"
)

// publicKeys: GET /v1/jwt/keys. Mapa versão -> chave pública Ed25519 (base64), só das versões
// que ainda validam tokens.
func (s *Server) publicKeys(w http.ResponseWriter, r *http.Request) {
	ks, err := s.Vault.PublicKeys(r.Context())
	if err != nil {
		s.vaultError(w, err)
		return
	}
	out := make(map[string]string, len(ks))
	for v, k := range ks {
		out[strconv.Itoa(v)] = b64(k)
	}
	writeJSON(w, out)
}
