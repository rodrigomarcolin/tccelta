package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"tccelta/backend/internal/casodeuso/porta"
)

// autenticarJWT chama o caso de uso de autenticação do Bearer e põe Sessao e Dongle no contexto.
func (d *Deps) autenticarJWT(exigirSubIgualID bool) mw {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Get("Authorization")
			if !strings.HasPrefix(h, "Bearer ") || len(h) < 8 {
				w.Header().Set("WWW-Authenticate", `Bearer realm="tccelta"`)
				problema(w, 401, "token_ausente", "Authorization: Bearer ausente", 0)
				return
			}
			a, err := d.Autenticar.Executar(r.Context(), strings.TrimSpace(h[7:]))
			if err != nil {
				switch {
				case errors.Is(err, porta.ErrTokenExpirado):
					w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token", error_description="expirado"`)
					problema(w, 401, "token_expirado", "token expirado", 0)
				case errors.Is(err, porta.ErrTokenInvalido):
					w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
					problema(w, 401, "token_invalido", "token inválido", 0)
				default:
					d.erro(w, err)
				}
				return
			}
			if exigirSubIgualID && !strings.EqualFold(r.PathValue("id"), a.Sessao.Dev.Hex()) {
				problema(w, 403, "token_outro_dongle", "o token pertence a outro dongle", 0)
				return
			}
			ctx := context.WithValue(r.Context(), ctxSessao, a.Sessao)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
