package server

import (
	"encoding/base64"
	"net/http"
	"strconv"
)

// signClientJWT: POST /v1/client-jwt/sign. Igual a /v1/jwt/sign, com a chave `cli` dos clientes.
func (s *Server) signClientJWT(w http.ResponseWriter, r *http.Request) {
	var req reqSign
	if !readJSON(w, r, &req) {
		return
	}
	input, err := base64.StdEncoding.Strict().DecodeString(req.Input)
	if err != nil || len(input) == 0 || len(input) > MaxInput {
		problem(w, 400, "entradas_invalidas", "entrada: base64 padrão de 1 a 4096 bytes esperado", 0)
		return
	}
	sig, version, err := s.Vault.SignClient(r.Context(), input)
	if err != nil {
		s.vaultError(w, err)
		return
	}
	writeJSON(w, respSign{Signature: b64(sig), Version: version})
}

// clientPublicKeys: GET /v1/client-jwt/keys. Mapa versão -> chave pública Ed25519 (base64) da chave `cli`.
func (s *Server) clientPublicKeys(w http.ResponseWriter, r *http.Request) {
	ks, err := s.Vault.ClientPublicKeys(r.Context())
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
