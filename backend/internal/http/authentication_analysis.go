package http

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"tccelta/backend/internal/port"
)

// authenticateAnalise exige o Bearer de client credentials nas rotas de viagem. As respostas são as de authenticateJWT
// (401 token_ausente, token_expirado, token_invalido, com WWW-Authenticate). O token do app (kid jwt-v<n>, aud de ingestão)
// não passa aqui, nem o de cliente passa na ingestão.
func (d *Deps) authenticateAnalise(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") || len(h) < 8 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="tccelta"`)
			problem(w, 401, "token_ausente", "Authorization: Bearer ausente", 0)
			return
		}
		s, err := d.AuthenticateClient.Run(r.Context(), strings.TrimSpace(h[7:]))
		if err != nil {
			switch {
			case errors.Is(err, port.ErrTokenExpired):
				w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token", error_description="expirado"`)
				problem(w, 401, "token_expirado", "token expirado", 0)
			case errors.Is(err, port.ErrInvalidToken):
				w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
				problem(w, 401, "token_invalido", "token inválido", 0)
			default:
				d.err(w, err)
			}
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxClient, s)))
	})
}
