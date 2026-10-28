package servidor

import (
	"net/http"
	"strconv"
)

// chavesPublicas: GET /v1/jwt/chaves. Mapa versão -> chave pública Ed25519 (base64), só das versões
// que ainda validam tokens.
func (s *Servidor) chavesPublicas(w http.ResponseWriter, r *http.Request) {
	ks, err := s.Vault.ChavesPublicas(r.Context())
	if err != nil {
		s.erroVault(w, err)
		return
	}
	out := make(map[string]string, len(ks))
	for v, k := range ks {
		out[strconv.Itoa(v)] = b64(k)
	}
	escrever(w, out)
}
