package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"tccelta/backend/internal/kms/vault"
)

// problem escreve application/problem+json (RFC 9457), no mesmo formato da API.
func problem(w http.ResponseWriter, status int, code, detail string, retryAfter time.Duration) {
	if retryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int((retryAfter+time.Second-1)/time.Second)))
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type": "https://tccelta.local/erros/kms/" + code, "title": code, "status": status, "codigo": code, "detail": detail})
}

// vaultError traduz o erro de uma operação sobre o Vault: indisponibilidade vira 503 kms_indisponivel
// com Retry-After; o que sobrar é erro interno.
func (s *Server) vaultError(w http.ResponseWriter, err error) {
	if errors.Is(err, vault.ErrUnavailable) {
		if s.Metrics != nil {
			s.Metrics.Reject("vault_indisponivel")
		}
		problem(w, 503, "kms_indisponivel", "Vault indisponível", 2*time.Second)
		return
	}
	s.Log.Error("http.erro_interno", "erro", err)
	problem(w, 500, "erro_interno", "erro interno", 0)
}
