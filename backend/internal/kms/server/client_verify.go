package server

import (
	"net/http"
	"tccelta/backend/internal/kms/vault"
)

// MaxSecret é o tamanho máximo do segredo do cliente (texto, ao contrário das demais entradas do contrato).
const MaxSecret = 128

type reqClientVerify struct {
	ClientID string `json:"client_id"`
	Secret   string `json:"secret"`
}

type respClientVerify struct {
	Valid bool `json:"valid"`
}

// verifyClient: POST /v1/clients/verify. Confere o segredo do cliente contra o hash guardado no Vault, em tempo
// constante. Segredo errado, cliente desconhecido e cliente revogado dão o mesmo `valid:false`.
func (s *Server) verifyClient(w http.ResponseWriter, r *http.Request) {
	var req reqClientVerify
	if !readJSON(w, r, &req) {
		return
	}
	if !vault.ValidClientID(req.ClientID) || len(req.Secret) < 1 || len(req.Secret) > MaxSecret {
		problem(w, 400, "entradas_invalidas", "client_id fora do formato ou secret de 1 a 128 caracteres esperado", 0)
		return
	}
	valid, err := s.Vault.VerifyClient(r.Context(), req.ClientID, req.Secret)
	if err != nil {
		s.vaultError(w, err)
		return
	}
	if s.Metrics != nil {
		res := "invalid"
		if valid {
			res = "valid"
		}
		s.Metrics.ClientVerify(res)
	}
	writeJSON(w, respClientVerify{Valid: valid})
}
