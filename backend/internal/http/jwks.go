package http

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"
)

type jwk struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	KID string `json:"kid"`
	X   string `json:"x"`
}

// jwks: GET /.well-known/jwks.json. Publica só as chaves cli-v<n>, nunca as jwt-v<n>, a partir do cache em memória do
// validador. Cache-Control public, max-age=300: menor que o ciclo de recarga e bem menor que a validade do token.
func (d *Deps) jwks(w http.ResponseWriter, r *http.Request) {
	keys, err := d.ClientKeys.Run(r.Context())
	if err != nil {
		problem(w, 503, "chaves_indisponiveis", "chaves públicas ainda não carregadas", time.Second)
		return
	}
	set := make([]jwk, len(keys))
	for i, k := range keys {
		set[i] = jwk{Kty: "OKP", Crv: "Ed25519", Alg: "EdDSA", Use: "sig", KID: k.KID, X: base64.RawURLEncoding.EncodeToString(k.Key)}
	}
	w.Header().Set("Content-Type", "application/jwk-set+json")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_ = json.NewEncoder(w).Encode(map[string]any{"keys": set})
}
