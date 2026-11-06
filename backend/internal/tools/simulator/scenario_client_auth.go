package simulator

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// ClientCred é um cliente de client credentials provisionado pelo `tccelta-kms client` para o cenário. Valid diz se o
// segredo do arquivo deve ser aceito (false: segredo antigo depois de `rotate`, ou cliente revogado).
type ClientCred struct {
	ID         string
	SecretFile string
	Valid      bool
}

// ParseClientCred lê "id:arquivo:ok" ou "id:arquivo:invalido" (o valor de --client).
func ParseClientCred(s string) (ClientCred, error) {
	p := strings.Split(s, ":")
	if len(p) != 3 || p[0] == "" || p[1] == "" || (p[2] != "ok" && p[2] != "invalido") {
		return ClientCred{}, fmt.Errorf("--client %q: use id:arquivo:ok ou id:arquivo:invalido", s)
	}
	return ClientCred{ID: p[0], SecretFile: p[1], Valid: p[2] == "ok"}, nil
}

// Secret lê o segredo do arquivo.
func (c ClientCred) Secret() (string, error) {
	b, err := os.ReadFile(c.SecretFile)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// primary devolve o primeiro cliente válido, com o segredo lido.
func primaryClient(o Options) (id, secret string, err error) {
	for _, c := range o.Clients {
		if c.Valid {
			s, err := c.Secret()
			return c.ID, s, err
		}
	}
	return "", "", fmt.Errorf("nenhum --client válido (id:arquivo:ok)")
}

const tokenForm = "grant_type=client_credentials"

type httpResult struct {
	status int
	header http.Header
	raw    string
	body   map[string]any
}

// doRaw faz a requisição e devolve status, cabeçalhos e corpo (JSON se houver).
func doRaw(ctx context.Context, hc *http.Client, method, url, contentType, authz, body string) (httpResult, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, strings.NewReader(body))
	if err != nil {
		return httpResult{}, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if authz != "" {
		req.Header.Set("Authorization", authz)
	}
	resp, err := hc.Do(req)
	if err != nil {
		return httpResult{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	r := httpResult{status: resp.StatusCode, header: resp.Header, raw: string(b)}
	_ = json.Unmarshal(b, &r.body)
	return r, nil
}

func basicAuth(id, secret string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(id+":"+secret))
}

// requestToken chama POST /v1/auth/token com Basic.
func requestToken(ctx context.Context, hc *http.Client, api, id, secret string) (httpResult, error) {
	return doRaw(ctx, hc, "POST", api+"/v1/auth/token", "application/x-www-form-urlencoded", basicAuth(id, secret), tokenForm)
}

// fetchToken obtém um token válido ou falha.
func fetchToken(ctx context.Context, hc *http.Client, api, id, secret string) (string, error) {
	r, err := requestToken(ctx, hc, api, id, secret)
	if err != nil {
		return "", err
	}
	tok := jstr(r.body, "access_token")
	if r.status != 200 || tok == "" {
		return "", fmt.Errorf("POST /v1/auth/token: %d %s", r.status, r.raw)
	}
	return tok, nil
}

// jwtParts decodifica header, claims e devolve a signing string e a assinatura de um JWT compacto.
func jwtParts(tok string) (hdr, claims map[string]any, signed string, sig []byte, ok bool) {
	p := strings.Split(tok, ".")
	if len(p) != 3 {
		return nil, nil, "", nil, false
	}
	h, e1 := base64.RawURLEncoding.DecodeString(p[0])
	c, e2 := base64.RawURLEncoding.DecodeString(p[1])
	s, e3 := base64.RawURLEncoding.DecodeString(p[2])
	if e1 != nil || e2 != nil || e3 != nil || json.Unmarshal(h, &hdr) != nil || json.Unmarshal(c, &claims) != nil {
		return nil, nil, "", nil, false
	}
	return hdr, claims, p[0] + "." + p[1], s, true
}

// jwksKeys lê GET /.well-known/jwks.json e devolve os JWK por kid.
func jwksKeys(ctx context.Context, hc *http.Client, api string) (httpResult, map[string]ed25519.PublicKey, error) {
	r, err := doRaw(ctx, hc, "GET", api+"/.well-known/jwks.json", "", "", "")
	if err != nil {
		return r, nil, err
	}
	out := map[string]ed25519.PublicKey{}
	ks, _ := r.body["keys"].([]any)
	for _, k := range ks {
		m, _ := k.(map[string]any)
		x, err := base64.RawURLEncoding.DecodeString(jstr(m, "x"))
		if err == nil && len(x) == ed25519.PublicKeySize && jstr(m, "kty") == "OKP" && jstr(m, "crv") == "Ed25519" {
			out[jstr(m, "kid")] = ed25519.PublicKey(x)
		}
	}
	return r, out, nil
}

func retryAfter(h http.Header) int { n, _ := strconv.Atoi(h.Get("Retry-After")); return n }

// metricLines devolve as linhas da exposição Prometheus que começam com prefix.
func metricLines(ctx context.Context, hc *http.Client, metricsURL, prefix string) []string {
	r, err := doRaw(ctx, hc, "GET", metricsURL, "", "", "")
	if err != nil {
		return nil
	}
	var out []string
	for _, l := range strings.Split(r.raw, "\n") {
		if strings.HasPrefix(l, prefix) {
			out = append(out, l)
		}
	}
	return out
}

func metricValue(lines []string, series string) float64 {
	for _, l := range lines {
		if strings.HasPrefix(l, series+" ") {
			f, _ := strconv.ParseFloat(strings.TrimSpace(strings.TrimPrefix(l, series+" ")), 64)
			return f
		}
	}
	return 0
}

// clientAuthScenario cobre POST /v1/auth/token, o JWKS, o Bearer das rotas de viagem, o isolamento entre o token do app
// e o de cliente, o bloqueio progressivo (Redis de verdade, duas ou mais réplicas se houver) e as métricas. Os clientes vêm
// de --client: o primeiro válido é o principal; os inválidos (segredo antigo após rotate, cliente revogado) precisam dar
// 401. A API do compose de teste usa uma progressão curta (TOKEN_BLOQUEIO_*). Com --token-file, grava o token emitido para a
// fase seguinte (client-auth-rotation).
func clientAuthScenario(ctx context.Context, o Options) error {
	v := &verifier{log: o.Log}
	hc := client()
	id, secret, err := primaryClient(o)
	if err != nil {
		return err
	}
	api := o.API

	// 1. sucesso
	r, err := requestToken(ctx, hc, api, id, secret)
	if err != nil {
		return err
	}
	tok := jstr(r.body, "access_token")
	v.ok(r.status == 200 && tok != "" && jstr(r.body, "token_type") == "Bearer" && jnum(r.body, "expires_in") > 0,
		"token emitido: %d, expires_in %.0f", r.status, jnum(r.body, "expires_in"))
	v.ok(r.header.Get("Cache-Control") == "no-store" && r.header.Get("Pragma") == "no-cache", "Cache-Control no-store e Pragma no-cache")
	hdr, claims, signed, sig, ok := jwtParts(tok)
	v.ok(ok && jstr(hdr, "alg") == "EdDSA" && strings.HasPrefix(jstr(hdr, "kid"), "cli-v") && jstr(claims, "aud") == "tccelta-analise" &&
		jstr(claims, "sub") == id && jstr(claims, "jti") != "" && jraw(claims, "cs") == nil, "header %v e claims %v", hdr, claims)

	// outros clientes válidos (por exemplo, o segredo novo depois de `client rotate`) também obtêm token
	for _, c := range o.Clients {
		if !c.Valid || c.ID == id {
			continue
		}
		s, err := c.Secret()
		if err != nil {
			return err
		}
		_, err = fetchToken(ctx, hc, api, c.ID, s)
		v.ok(err == nil, "cliente %s com o segredo vigente obtém token (%v)", c.ID, err)
	}

	// 2. JWKS: só chaves cli, e o token verifica com a chave do kid
	jr, keys, err := jwksKeys(ctx, hc, api)
	if err != nil {
		return err
	}
	v.ok(jr.status == 200 && strings.HasPrefix(jr.header.Get("Content-Type"), "application/jwk-set+json") && jr.header.Get("Cache-Control") == "public, max-age=300",
		"JWKS: %d %s %s", jr.status, jr.header.Get("Content-Type"), jr.header.Get("Cache-Control"))
	onlyCli := len(keys) > 0
	for kid := range keys {
		onlyCli = onlyCli && strings.HasPrefix(kid, "cli-v")
	}
	v.ok(onlyCli && !strings.Contains(jr.raw, "jwt-v"), "JWKS só com chaves cli: %d chave(s)", len(keys))
	pub := keys[jstr(hdr, "kid")]
	v.ok(len(pub) == ed25519.PublicKeySize && ed25519.Verify(pub, []byte(signed), sig), "o token verifica com a chave %s do JWKS", jstr(hdr, "kid"))
	if o.TokenFile != "" {
		if err := os.WriteFile(o.TokenFile, []byte(tok), 0o600); err != nil {
			return err
		}
	}

	// 3. o Bearer nas rotas de viagem
	trips := &tripAPI{base: api, http: hc}
	missing := "/v1/viagens/6f1c1f7e-3f0a-4a53-9d51-0c1d7a1b2c3d"
	st, body, hd := trips.do(ctx, "GET", missing, "")
	v.ok(st == 401 && jstr(body, "codigo") == "token_ausente" && hd.Get("WWW-Authenticate") != "", "rota de viagem sem Bearer: %d %s", st, jstr(body, "codigo"))
	trips.token = "isto.nao.eJwt"
	st, body, _ = trips.do(ctx, "GET", missing, "")
	v.ok(st == 401 && jstr(body, "codigo") == "token_invalido", "rota de viagem com token inválido: %d %s", st, jstr(body, "codigo"))
	trips.token = tok
	st, body, _ = trips.do(ctx, "GET", missing, "")
	v.ok(st == 404 && jstr(body, "codigo") == "viagem_desconhecida", "rota de viagem com token válido passa pela autenticação: %d %s", st, jstr(body, "codigo"))

	// 4. o token de cliente não vale na ingestão
	ir, err := doRaw(ctx, hc, "POST", api+"/v1/dongles/blocos", "application/octet-stream", "Bearer "+tok, "x")
	if err != nil {
		return err
	}
	v.ok(ir.status == 401, "token de cliente em /v1/dongles/blocos: %d %s", ir.status, jstr(ir.body, "codigo"))
	cr, err := doRaw(ctx, hc, "GET", api+"/v1/dongles/0011223344556677/confirmacao", "", "Bearer "+tok, "")
	if err != nil {
		return err
	}
	v.ok(cr.status == 401, "token de cliente em /v1/dongles/{id}/confirmacao: %d", cr.status)

	// 5. erros 400 (RFC 6749 §5.2), sem contar como falha
	for _, c := range []struct{ name, ct, body, oauth string }{
		{"grant_type errado", "application/x-www-form-urlencoded", "grant_type=password", "unsupported_grant_type"},
		{"scope", "application/x-www-form-urlencoded", tokenForm + "&scope=x", "invalid_scope"},
		{"Content-Type", "application/json", "{}", "invalid_request"},
	} {
		br, err := doRaw(ctx, hc, "POST", api+"/v1/auth/token", c.ct, basicAuth(id, secret), c.body)
		if err != nil {
			return err
		}
		v.ok(br.status == 400 && jstr(br.body, "error") == c.oauth && br.header.Get("Cache-Control") == "no-store", "%s: %d %s", c.name, br.status, jstr(br.body, "error"))
	}

	// 6. invalid_client uniforme: segredo errado, cliente desconhecido, sem Basic e os --client inválidos
	type attempt struct{ name, id, secret string }
	attempts := []attempt{{"segredo errado", id, strings.Repeat("a", 43)}, {"cliente desconhecido", "nao-existe-e2e", secret}}
	for _, c := range o.Clients {
		if !c.Valid {
			s, err := c.Secret()
			if err != nil {
				return err
			}
			attempts = append(attempts, attempt{"cliente " + c.ID + " (segredo antigo ou revogado)", c.ID, s})
		}
	}
	var refBody, refWWW string
	uniform := true
	for _, a := range attempts {
		ar, err := requestToken(ctx, hc, api, a.id, a.secret)
		if err != nil {
			return err
		}
		if refBody == "" {
			refBody, refWWW = ar.raw, ar.header.Get("WWW-Authenticate")
		}
		same := ar.status == 401 && ar.raw == refBody && ar.header.Get("WWW-Authenticate") == refWWW && refWWW != "" &&
			jstr(ar.body, "error") == "invalid_client" && ar.header.Get("Cache-Control") == "no-store"
		uniform = uniform && same
		v.ok(same, "%s: %d %s", a.name, ar.status, jstr(ar.body, "error"))
	}
	nr, err := doRaw(ctx, hc, "POST", api+"/v1/auth/token", "application/x-www-form-urlencoded", "", tokenForm)
	if err != nil {
		return err
	}
	v.ok(nr.status == 401 && nr.raw == refBody && uniform, "sem Basic: mesma resposta (%d)", nr.status)

	// 7. bloqueio progressivo: o par (id, IP) tem tolerância de 5 falhas; a 6ª é 401 e a seguinte, 429 sem chamar o KMS.
	// Um sucesso antes zera o contador do par (o do IP segue).
	if _, err := fetchToken(ctx, hc, api, id, secret); err != nil {
		return err
	}
	var last httpResult
	for i := 0; i < 6; i++ {
		if last, err = requestToken(ctx, hc, api, id, "errado-"+strconv.Itoa(i)); err != nil {
			return err
		}
	}
	v.ok(last.status == 401, "a 6ª falha do par ainda é 401 (%d)", last.status)
	br, err := requestToken(ctx, hc, api, id, secret) // a credencial certa também espera
	if err != nil {
		return err
	}
	ra := retryAfter(br.header)
	v.ok(br.status == 429 && ra >= 1 && jstr(br.body, "error") == "temporarily_unavailable" && jstr(br.body, "codigo") == "bloqueado" &&
		br.header.Get("Cache-Control") == "no-store", "bloqueado: %d, Retry-After %d s", br.status, ra)
	time.Sleep(time.Duration(ra)*time.Second + 500*time.Millisecond)
	br, err = requestToken(ctx, hc, api, id, secret)
	if err != nil {
		return err
	}
	v.ok(br.status == 200, "depois do Retry-After a credencial certa volta a funcionar (%d)", br.status)

	// 8. métricas: as séries existem e nenhuma tem client_id ou IP como rótulo
	lines := metricLines(ctx, hc, o.MetricsURL, "tccelta_auth_")
	v.ok(metricValue(lines, `tccelta_auth_token_requests_total{result="issued"}`) >= 1 &&
		metricValue(lines, `tccelta_auth_token_requests_total{result="invalid_client"}`) >= 1 &&
		metricValue(lines, `tccelta_auth_token_requests_total{result="blocked"}`) >= 1 &&
		metricValue(lines, `tccelta_auth_token_requests_total{result="invalid_request"}`) >= 1 &&
		metricValue(lines, `tccelta_auth_blocks_total{scope="pair"}`) >= 1, "métricas tccelta_auth_*: %d série(s)", len(lines))
	noLabels := len(lines) > 0
	for _, l := range lines {
		noLabels = noLabels && !strings.Contains(l, "client_id") && !strings.Contains(l, id) && !strings.Contains(l, "ip=")
	}
	v.ok(noLabels, "nenhuma série usa client_id ou IP como rótulo")
	return v.result()
}

// clientAuthRotationScenario roda depois de `tccelta-kms clientjwt rotate --now`: o token da fase anterior segue válido até exp,
// o token novo sai com a chave cli-v<n+1> e o JWKS passa a publicar as duas, sem reiniciar a API.
func clientAuthRotationScenario(ctx context.Context, o Options) error {
	v := &verifier{log: o.Log}
	hc := client()
	id, secret, err := primaryClient(o)
	if err != nil {
		return err
	}
	oldTok, err := os.ReadFile(o.TokenFile)
	if err != nil {
		return fmt.Errorf("--token-file da fase anterior: %w", err)
	}
	oldHdr, _, _, _, _ := jwtParts(strings.TrimSpace(string(oldTok)))
	trips := &tripAPI{base: o.API, http: hc}
	missing := "/v1/viagens/6f1c1f7e-3f0a-4a53-9d51-0c1d7a1b2c3d"

	// o token novo é assinado pela versão nova; o validador da API recarrega as chaves ao ver o kid desconhecido (no máximo a cada 30 s)
	newTok, err := fetchToken(ctx, hc, o.API, id, secret)
	if err != nil {
		return err
	}
	newHdr, _, _, _, _ := jwtParts(newTok)
	v.ok(jstr(newHdr, "kid") != jstr(oldHdr, "kid") && strings.HasPrefix(jstr(newHdr, "kid"), "cli-v"), "kid novo %s (antes %s)", jstr(newHdr, "kid"), jstr(oldHdr, "kid"))
	trips.token = newTok
	deadline := time.Now().Add(45 * time.Second)
	st := 0
	for {
		st, _, _ = trips.do(ctx, "GET", missing, "")
		if st != 401 || time.Now().After(deadline) {
			break
		}
		time.Sleep(2 * time.Second)
	}
	v.ok(st == 404, "o token com a chave nova é aceito sem reiniciar a API (%d)", st)
	trips.token = strings.TrimSpace(string(oldTok))
	st, _, _ = trips.do(ctx, "GET", missing, "")
	v.ok(st == 404, "o token da versão anterior segue válido até exp (%d)", st)

	_, keys, err := jwksKeys(ctx, hc, o.API)
	if err != nil {
		return err
	}
	_, hasOld := keys[jstr(oldHdr, "kid")]
	_, hasNew := keys[jstr(newHdr, "kid")]
	v.ok(hasOld && hasNew, "o JWKS publica as duas versões: %d chave(s)", len(keys))
	return v.result()
}
