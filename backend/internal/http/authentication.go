package http

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"tccelta/backend/internal/port"
)

// authenticateJWT chama o caso de uso de autenticação do Bearer e põe Session e Dongle no contexto.
func (d *Deps) authenticateJWT(requireSubEqualID bool) mw {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Get("Authorization")
			if !strings.HasPrefix(h, "Bearer ") || len(h) < 8 {
				w.Header().Set("WWW-Authenticate", `Bearer realm="tccelta"`)
				problem(w, 401, "token_ausente", "Authorization: Bearer ausente", 0)
				return
			}
			a, err := d.Authenticate.Run(r.Context(), strings.TrimSpace(h[7:]))
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
			if requireSubEqualID && !strings.EqualFold(r.PathValue("id"), a.Session.Dev.Hex()) {
				problem(w, 403, "token_outro_dongle", "o token pertence a outro dongle", 0)
				return
			}
			ctx := context.WithValue(r.Context(), ctxSession, a.Session)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
