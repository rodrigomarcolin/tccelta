package servidor

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"tccelta/kms/internal/vault"
)

// problema escreve application/problem+json (RFC 9457), no mesmo formato da API.
func problema(w http.ResponseWriter, status int, codigo, detalhe string, retryAfter time.Duration) {
	if retryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int((retryAfter+time.Second-1)/time.Second)))
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type": "https://tccelta.local/erros/kms/" + codigo, "title": codigo, "status": status, "codigo": codigo, "detail": detalhe})
}

// erroVault traduz o erro de uma operação sobre o Vault: indisponibilidade vira 503 kms_indisponivel
// com Retry-After; o que sobrar é erro interno.
func (s *Servidor) erroVault(w http.ResponseWriter, err error) {
	if errors.Is(err, vault.ErrIndisponivel) {
		if s.Met != nil {
			s.Met.Rejeicao("vault_indisponivel")
		}
		problema(w, 503, "kms_indisponivel", "Vault indisponível", 2*time.Second)
		return
	}
	s.Log.Error("http.erro_interno", "erro", err)
	problema(w, 500, "erro_interno", "erro interno", 0)
}
