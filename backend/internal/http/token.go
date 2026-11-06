package http

import (
	"encoding/json"
	"errors"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"tccelta/backend/internal/domain/clientauth"
	"tccelta/backend/internal/port"
	"tccelta/backend/internal/usecase/clienttoken"
)

// oauthError escreve o erro no formato da RFC 6749 §5.2 ({"error","error_description"}), mais o `codigo` do projeto. Só
// POST /v1/auth/token usa este formato; as demais rotas seguem problem.go.
func oauthError(w http.ResponseWriter, status int, oauth, code, description string, retryAfter time.Duration) {
	if retryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int((retryAfter+time.Second-1)/time.Second)))
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": oauth, "error_description": description, "codigo": code})
}

// noStore põe Cache-Control: no-store e Pragma: no-cache em todas as respostas da rota, inclusive as de erro e as dos
// middlewares seguintes (RFC 6749 §5.1).
func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Pragma", "no-cache")
		next.ServeHTTP(w, r)
	})
}

// clientIP é o IP usado pelo bloqueio progressivo. Parte de ipFrom (que só confia em X-Real-IP vindo de PROXIES_CONFIAVEIS)
// e fecha as lacunas dele: conteúdo que não é IP cai no host de RemoteAddr; IPv4 mapeado em IPv6 vira IPv4; IPv6 é agrupado
// por /64, para que quem troca de endereço dentro do prefixo não fuja do contador.
func (d *Deps) clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	a, err := netip.ParseAddr(strings.TrimSpace(d.ipFrom(r)))
	if err != nil {
		if a, err = netip.ParseAddr(host); err != nil {
			return host
		}
	}
	a = a.WithZone("").Unmap()
	if a.Is6() {
		return netip.PrefixFrom(a, 64).Masked().String()
	}
	return a.String()
}

const (
	grantClientCredentials = "client_credentials"
	basicRealm             = `Basic realm="tccelta"`
)

// issueToken: POST /v1/auth/token (RFC 6749 §4.4). Credenciais só por HTTP Basic; client_id e client_secret no corpo são
// ignorados. Erros 400 e 503 não chamam o bloqueio nem contam como falha.
func (d *Deps) issueToken(w http.ResponseWriter, r *http.Request) {
	result := func(res string) {
		if d.MetAuth != nil {
			d.MetAuth.TokenRequest(res)
		}
	}
	invalid := func(oauth, code, desc string) {
		result("invalid_request")
		oauthError(w, 400, oauth, code, desc, 0)
	}
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/x-www-form-urlencoded" {
		invalid("invalid_request", "requisicao_invalida", "Content-Type deve ser application/x-www-form-urlencoded")
		return
	}
	if err := r.ParseForm(); err != nil {
		invalid("invalid_request", "requisicao_invalida", "corpo inválido ou acima de 1 KiB")
		return
	}
	grants := r.PostForm["grant_type"]
	switch {
	case len(grants) > 1:
		invalid("invalid_request", "requisicao_invalida", "grant_type repetido")
		return
	case len(grants) == 0 || grants[0] != grantClientCredentials:
		invalid("unsupported_grant_type", "grant_invalido", "grant_type deve ser client_credentials")
		return
	}
	if _, has := r.PostForm["scope"]; has {
		invalid("invalid_scope", "escopo_invalido", "scope não é aceito")
		return
	}

	id, secret, present := r.BasicAuth()
	tok, err := d.IssueToken.Run(r.Context(), clienttoken.Credentials{Present: present, ClientID: id, Secret: secret, IP: d.clientIP(r)})
	switch {
	case err == nil:
		result("issued")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": tok.AccessToken, "token_type": "Bearer", "expires_in": int(tok.ExpiresIn.Seconds())})
	case errors.Is(err, clientauth.ErrBlocked):
		var be clientauth.BlockedError
		_ = errors.As(err, &be)
		result("blocked")
		d.rej("token_bloqueado")
		oauthError(w, 429, "temporarily_unavailable", "bloqueado", "muitas tentativas; tente depois do tempo indicado", be.RetryAfter)
	case errors.Is(err, clientauth.ErrInvalidClient):
		result("invalid_client")
		w.Header().Set("WWW-Authenticate", basicRealm)
		oauthError(w, 401, "invalid_client", "cliente_invalido", "credenciais do cliente inválidas", 0)
	case errors.Is(err, port.ErrKMSUnavailable):
		result("kms_unavailable")
		d.rej("kms")
		oauthError(w, 503, "temporarily_unavailable", "kms_indisponivel", "KMS indisponível", d.retry503())
	default:
		d.Log.Error("http.erro_interno", "erro", err)
		oauthError(w, 500, "server_error", "erro_interno", "erro interno", 0)
	}
}
