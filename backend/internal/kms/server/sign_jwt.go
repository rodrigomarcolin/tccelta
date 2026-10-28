package server

import (
	"encoding/base64"
	"net/http"
)

type reqSign struct {
	Input string `json:"input"`
}

type respSign struct {
	Signature string `json:"signature"`
	Version   int    `json:"version"`
}

// signJWT: POST /v1/jwt/sign. Assina a signing string do JWT com a chave Ed25519; devolve a
// signature (64 B) e a versão da chave que assinou.
func (s *Server) signJWT(w http.ResponseWriter, r *http.Request) {
	var req reqSign
	if !readJSON(w, r, &req) {
		return
	}
	input, err := base64.StdEncoding.Strict().DecodeString(req.Input)
	if err != nil || len(input) == 0 || len(input) > MaxInput {
		problem(w, 400, "entradas_invalidas", "entrada: base64 padrão de 1 a 4096 bytes esperado", 0)
		return
	}
	sig, version, err := s.Vault.Sign(r.Context(), input)
	if err != nil {
		s.vaultError(w, err)
		return
	}
	writeJSON(w, respSign{Signature: b64(sig), Version: version})
}
