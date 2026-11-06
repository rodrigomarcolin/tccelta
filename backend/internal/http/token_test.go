package http

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"tccelta/backend/internal/adapter/authblock"
	"tccelta/backend/internal/adapter/jwt"
	"tccelta/backend/internal/domain/clientauth"
	"tccelta/backend/internal/domain/protocol"
	"tccelta/backend/internal/domain/trip"
	"tccelta/backend/internal/http/ratelimit"
	"tccelta/backend/internal/port"
	"tccelta/backend/internal/port/porttest"
	clientUC "tccelta/backend/internal/usecase/clienttoken"
)

const (
	testClient = "next-dev"
	testSecret = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFG"
	formOK     = "grant_type=client_credentials"
	formCT     = "application/x-www-form-urlencoded"
)

var authPolicies = authblock.Policies{
	Pair: clientauth.Policy{Tolerance: 5, Initial: 30 * time.Second, Factor: 2, Cap: 15 * time.Minute, Forget: 30 * time.Minute},
	IP:   clientauth.Policy{Tolerance: 20, Initial: 30 * time.Second, Factor: 2, Cap: 15 * time.Minute, Forget: 30 * time.Minute},
}

// authRecorder guarda os eventos da porta de métricas do client credentials.
type authRecorder struct {
	porttest.Nop
	results       []string
	blocks        []string
	storeFailures int
}

func (a *authRecorder) TokenRequest(r string) { a.results = append(a.results, r) }
func (a *authRecorder) Block(s string)        { a.blocks = append(a.blocks, s) }
func (a *authRecorder) BlockStoreFailure()    { a.storeFailures++ }

func (a *authRecorder) count(r string) int {
	n := 0
	for _, x := range a.results {
		if x == r {
			n++
		}
	}
	return n
}

// authEnv é uma réplica da API com o client credentials de verdade (casos de uso, emissor e validador sobre o KMS em
// memória, bloqueio no Redis de teste) e os casos de uso de viagem falsos.
type authEnv struct {
	*tripEnv
	kms *porttest.Memory
	mr  *miniredis.Miniredis
	met *authRecorder
	val *jwt.ClientValidator
	h   http.Handler
}

func newAuthEnv(t *testing.T, mr *miniredis.Miniredis, kms *porttest.Memory) *authEnv {
	t.Helper()
	e := &authEnv{tripEnv: newTripEnv(t, map[string]string{"PROXIES_CONFIAVEIS": "172.30.10.0/24"}), kms: kms, mr: mr, met: &authRecorder{}}
	e.d.Cfg.HTTP.TrustedProxies = []string{"172.30.10.0/24"}
	val, err := jwt.NewClientValidator(context.Background(), kms, "iss", "tccelta-analise", 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	e.val = val
	var rc authblock.Redis
	if mr != nil {
		c := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1, DialTimeout: 100 * time.Millisecond})
		t.Cleanup(func() { _ = c.Close() })
		rc = c
	}
	blocker := authblock.New(rc, 300*time.Millisecond, authPolicies, e.met)
	e.d.IssueToken = clientUC.NewIssueToken(kms, jwt.NewClientIssuer(kms, "iss", "tccelta-analise", 30*time.Minute), blocker, e.met,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	e.d.AuthenticateClient = clientUC.NewAuthenticate(val)
	e.d.ClientKeys = clientUC.NewGetKeys(val)
	e.d.MetAuth = e.met
	e.h = e.d.Router()
	return e
}

func newKMS() *porttest.Memory {
	k := porttest.New()
	k.AddClient(testClient, testSecret)
	return k
}

func basic(id, secret string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(id+":"+secret))
}

// reqFrom envia a requisição como se viesse de remote (host:porta).
func reqFrom(h http.Handler, remote, method, path, body string, hdr map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.RemoteAddr = remote
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// token pede o token com Basic e o corpo dados, do IP remote.
func (e *authEnv) token(remote, authz, body string) *httptest.ResponseRecorder {
	hdr := map[string]string{"Content-Type": formCT}
	if authz != "" {
		hdr["Authorization"] = authz
	}
	return reqFrom(e.h, remote, "POST", "/v1/auth/token", body, hdr)
}

func (e *authEnv) bad(remote string) *httptest.ResponseRecorder {
	return e.token(remote, basic(testClient, "errado"), formOK)
}

func (e *authEnv) good(remote string) *httptest.ResponseRecorder {
	return e.token(remote, basic(testClient, testSecret), formOK)
}

func advanceRedis(mr *miniredis.Miniredis, now *time.Time, d time.Duration) {
	*now = now.Add(d)
	mr.SetTime(*now)
	mr.FastForward(d)
}

func oauthBody(t *testing.T, w *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	var m map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("%v: %s", err, w.Body.String())
	}
	return m
}

func assertNoStore(t *testing.T, w *httptest.ResponseRecorder, what string) {
	t.Helper()
	if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Pragma") != "no-cache" {
		t.Errorf("%s: sem no-store/no-cache: %v", what, w.Header())
	}
}

func TestTokenSuccess(t *testing.T) {
	kms := newKMS()
	e := newAuthEnv(t, miniredis.RunT(t), kms)
	w := e.good("192.0.2.1:5000")
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/json" {
		t.Fatal(w.Code, w.Header(), w.Body.String())
	}
	assertNoStore(t, w, "sucesso")
	var out struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.TokenType != "Bearer" || out.ExpiresIn != 1800 || out.AccessToken == "" {
		t.Fatalf("%+v", out)
	}
	// o token verifica com a chave do JWKS e é aceito nas rotas de viagem
	parts := strings.Split(out.AccessToken, ".")
	hd, _ := base64.RawURLEncoding.DecodeString(parts[0])
	cl, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var h, c map[string]any
	_ = json.Unmarshal(hd, &h)
	_ = json.Unmarshal(cl, &c)
	if h["alg"] != "EdDSA" || h["kid"] != "cli-v1" || c["aud"] != "tccelta-analise" || c["sub"] != testClient || c["iss"] != "iss" {
		t.Fatalf("%v %v", h, c)
	}
	jw := reqFrom(e.h, "192.0.2.1:5000", "GET", "/.well-known/jwks.json", "", nil)
	var set struct {
		Keys []jwk `json:"keys"`
	}
	_ = json.Unmarshal(jw.Body.Bytes(), &set)
	x, err := base64.RawURLEncoding.DecodeString(set.Keys[0].X)
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !ed25519.Verify(ed25519.PublicKey(x), []byte(parts[0]+"."+parts[1]), sig) {
		t.Fatal("o JWT não verifica com a chave do JWKS")
	}
	e.create.out = trip.Trip{ID: tripID, Status: trip.StatusPending}
	if tw := req(e.h, "POST", "/v1/viagens", []byte(validBody), map[string]string{"Content-Type": "application/json", "Authorization": "Bearer " + out.AccessToken}); tw.Code != 202 {
		t.Fatal(tw.Code, tw.Body.String())
	}
	if got := e.met.results; len(got) != 1 || got[0] != "issued" {
		t.Fatalf("%v", got)
	}
}

func TestTokenBodyCredentialsAreIgnored(t *testing.T) {
	e := newAuthEnv(t, miniredis.RunT(t), newKMS())
	// client_id/client_secret no corpo não valem: só o Basic
	w := e.token("192.0.2.1:5000", basic(testClient, testSecret), formOK+"&client_id=outro&client_secret=errado")
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	w = e.token("192.0.2.1:5000", "", formOK+"&client_id="+testClient+"&client_secret="+testSecret)
	if w.Code != 401 {
		t.Fatalf("credenciais no corpo, sem Basic: %d", w.Code)
	}
	// a query string também não conta
	w = reqFrom(e.h, "192.0.2.1:5000", "POST", "/v1/auth/token?scope=x", formOK, map[string]string{"Content-Type": formCT, "Authorization": basic(testClient, testSecret)})
	if w.Code != 200 {
		t.Fatalf("scope na query não é scope: %d %s", w.Code, w.Body.String())
	}
}

func TestTokenBadRequests(t *testing.T) {
	kms := newKMS()
	e := newAuthEnv(t, miniredis.RunT(t), kms)
	ok := basic(testClient, testSecret)
	cases := []struct {
		name, ct, body string
		status         int
		oauth, codigo  string
	}{
		{"sem Content-Type", "", formOK, 400, "invalid_request", "requisicao_invalida"},
		{"JSON", "application/json", `{"grant_type":"client_credentials"}`, 400, "invalid_request", "requisicao_invalida"},
		{"corpo acima de 1 KiB", formCT, formOK + "&x=" + strings.Repeat("a", 1100), 400, "invalid_request", "requisicao_invalida"},
		{"corpo malformado", formCT, "grant_type=%zz", 400, "invalid_request", "requisicao_invalida"},
		{"grant repetido", formCT, formOK + "&grant_type=client_credentials", 400, "invalid_request", "requisicao_invalida"},
		{"sem grant_type", formCT, "", 400, "unsupported_grant_type", "grant_invalido"},
		{"grant_type password", formCT, "grant_type=password", 400, "unsupported_grant_type", "grant_invalido"},
		{"scope informado", formCT, formOK + "&scope=leitura", 400, "invalid_scope", "escopo_invalido"},
		{"scope vazio", formCT, formOK + "&scope=", 400, "invalid_scope", "escopo_invalido"},
	}
	for i := 0; i < 3; i++ { // repetir não pode virar bloqueio: erros 400 não contam como falha
		for _, c := range cases {
			hdr := map[string]string{"Authorization": ok}
			if c.ct != "" {
				hdr["Content-Type"] = c.ct
			}
			w := reqFrom(e.h, "192.0.2.9:1", "POST", "/v1/auth/token", c.body, hdr)
			b := oauthBody(t, w)
			if w.Code != c.status || b["error"] != c.oauth || b["codigo"] != c.codigo || b["error_description"] == "" ||
				w.Header().Get("Content-Type") != "application/json" {
				t.Errorf("%s: %d %s", c.name, w.Code, w.Body.String())
			}
			assertNoStore(t, w, c.name)
		}
	}
	if kms.VerifyClientCalls.Load() != 0 {
		t.Fatal("erro 400 não chama o KMS")
	}
	if w := e.token("192.0.2.9:1", ok, formOK); w.Code != 200 {
		t.Fatalf("depois dos 400, o IP segue livre: %d", w.Code)
	}
	if e.met.count("invalid_request") != 3*len(cases) {
		t.Fatalf("%v", e.met.results)
	}
	if len(e.met.blocks) != 0 {
		t.Fatalf("%v", e.met.blocks)
	}
}

// Sem Basic, Basic malformado, cliente desconhecido, revogado e segredo errado: mesmo status, corpo e cabeçalhos.
func TestInvalidClientIsUniform(t *testing.T) {
	kms := newKMS()
	kms.AddClient("velho-dev", testSecret)
	kms.RevokeClient("velho-dev")
	e := newAuthEnv(t, miniredis.RunT(t), kms)
	notBasic := "Bearer abc"
	cases := map[string]string{
		"sem Authorization":     "",
		"esquema errado":        notBasic,
		"Basic sem base64":      "Basic !!!",
		"Basic sem dois-pontos": "Basic " + base64.StdEncoding.EncodeToString([]byte("semdoispontos")),
		"segredo errado":        basic(testClient, "errado"),
		"cliente desconhecido":  basic("outro-dev", testSecret),
		"cliente revogado":      basic("velho-dev", testSecret),
		"id malformado":         basic("Next Dev", testSecret),
		"segredo vazio":         basic(testClient, ""),
	}
	var refBody, refWWW string
	for name, authz := range cases {
		w := e.token("192.0.2.1:1", authz, formOK)
		if w.Code != 401 {
			t.Fatalf("%s: %d %s", name, w.Code, w.Body.String())
		}
		b := oauthBody(t, w)
		if b["error"] != "invalid_client" || b["codigo"] != "cliente_invalido" {
			t.Errorf("%s: %v", name, b)
		}
		if refBody == "" {
			refBody, refWWW = w.Body.String(), w.Header().Get("WWW-Authenticate")
		}
		if w.Body.String() != refBody || w.Header().Get("WWW-Authenticate") != refWWW || refWWW != `Basic realm="tccelta"` {
			t.Errorf("%s: resposta diferente das outras: %q %q", name, w.Body.String(), w.Header().Get("WWW-Authenticate"))
		}
		assertNoStore(t, w, name)
		if w.Header().Get("Content-Type") != "application/json" || w.Header().Get("Retry-After") != "" {
			t.Errorf("%s: %v", name, w.Header())
		}
		// cada IP começa limpo: nenhuma resposta depende do IP já ter falhado
		e.mr.FlushAll()
	}
}

// A 6ª falha do par é 401, a 7ª tentativa é 429 com Retry-After 1..30 e o KMS não recebe chamada; a falha seguinte ao fim do bloqueio gera 60 s.
func TestProgressiveBlockOverHTTP(t *testing.T) {
	kms := newKMS()
	mr := miniredis.RunT(t)
	now := time.Unix(1_800_000_000, 0)
	mr.SetTime(now)
	e := newAuthEnv(t, mr, kms)
	const ip = "192.0.2.50:4000"
	for i := 1; i <= 6; i++ {
		if w := e.bad(ip); w.Code != 401 {
			t.Fatalf("falha %d: %d", i, w.Code)
		}
	}
	calls := kms.VerifyClientCalls.Load()
	w := e.bad(ip)
	ra, _ := strconv.Atoi(w.Header().Get("Retry-After"))
	b := oauthBody(t, w)
	if w.Code != 429 || ra < 1 || ra > 30 || b["error"] != "temporarily_unavailable" || b["codigo"] != "bloqueado" {
		t.Fatalf("7ª tentativa: %d %v %s", w.Code, w.Header(), w.Body.String())
	}
	assertNoStore(t, w, "429")
	// a credencial certa também espera, e as tentativas bloqueadas não prolongam o bloqueio
	advanceRedis(mr, &now, 10*time.Second)
	for i := 0; i < 5; i++ {
		e.good(ip)
		e.bad(ip)
	}
	if kms.VerifyClientCalls.Load() != calls {
		t.Fatal("durante o bloqueio o KMS não recebe chamada")
	}
	w = e.good(ip)
	if ra, _ = strconv.Atoi(w.Header().Get("Retry-After")); w.Code != 429 || ra > 20 {
		t.Fatalf("o bloqueio não foi prolongado (restam ≤ 20 s): %d %d", w.Code, ra)
	}
	// fim do bloqueio: a próxima falha (7ª) bloqueia por 60 s
	advanceRedis(mr, &now, 21*time.Second)
	if w := e.bad(ip); w.Code != 401 {
		t.Fatalf("depois do bloqueio: %d", w.Code)
	}
	w = e.good(ip)
	if ra, _ = strconv.Atoi(w.Header().Get("Retry-After")); w.Code != 429 || ra < 31 || ra > 60 {
		t.Fatalf("7ª falha bloqueia por 60 s: %d %d", w.Code, ra)
	}
	// até o teto de 15 min
	for _, wait := range []time.Duration{60, 120, 240, 480, 900, 900} {
		advanceRedis(mr, &now, wait*time.Second+time.Second)
		e.bad(ip)
	}
	w = e.good(ip)
	if ra, _ = strconv.Atoi(w.Header().Get("Retry-After")); w.Code != 429 || ra < 890 || ra > 900 {
		t.Fatalf("teto de 15 min: %d %d", w.Code, ra)
	}
	if e.met.count("blocked") == 0 || e.met.count("invalid_client") == 0 || len(e.met.blocks) == 0 || e.met.blocks[0] != "pair" {
		t.Fatalf("%v %v", e.met.results, e.met.blocks)
	}
}

func TestIPBlockAndIsolation(t *testing.T) {
	kms := newKMS()
	e := newAuthEnv(t, miniredis.RunT(t), kms)
	const ip = "192.0.2.60:4000"
	for i := 1; i <= 21; i++ {
		id := "cli-" + strconv.Itoa(100+i)
		if w := e.token(ip, basic(id, "errado"), formOK); w.Code != 401 {
			t.Fatalf("falha %d: %d", i, w.Code)
		}
	}
	// 21 falhas do IP, nenhum par passou da tolerância: o IP está bloqueado, até para o cliente real
	if w := e.good(ip); w.Code != 429 {
		t.Fatalf("IP bloqueado: %d", w.Code)
	}
	// outro IP segue obtendo token
	if w := e.good("192.0.2.61:4000"); w.Code != 200 {
		t.Fatalf("outro IP: %d", w.Code)
	}
	if got := e.met.blocks; len(got) != 1 || got[0] != "ip" {
		t.Fatalf("%v", got)
	}
}

func TestPairBlockDoesNotAffectOtherIP(t *testing.T) {
	e := newAuthEnv(t, miniredis.RunT(t), newKMS())
	for i := 0; i < 6; i++ {
		e.bad("192.0.2.70:1")
	}
	if w := e.good("192.0.2.70:1"); w.Code != 429 {
		t.Fatalf("(a, IP1) bloqueado: %d", w.Code)
	}
	if w := e.good("192.0.2.71:1"); w.Code != 200 {
		t.Fatalf("(a, IP2): %d", w.Code)
	}
}

func TestSuccessResetsPairKeepsIPOverHTTP(t *testing.T) {
	e := newAuthEnv(t, miniredis.RunT(t), newKMS())
	const ip = "192.0.2.80:1"
	for i := 0; i < 5; i++ {
		e.bad(ip)
	}
	if w := e.good(ip); w.Code != 200 {
		t.Fatal(w.Code)
	}
	for i := 0; i < 5; i++ {
		e.bad(ip) // o par recomeçou do zero: 5 falhas, ainda dentro da tolerância
	}
	if w := e.good(ip); w.Code != 200 {
		t.Fatalf("o sucesso zerou o par: %d", w.Code)
	}
	if v := e.mr.HGet("tccelta:tok:i:192.0.2.80", "n"); v != "10" {
		t.Fatalf("o contador do IP segue: %q", v)
	}
	if e.mr.Exists("tccelta:tok:p:" + testClient + "|192.0.2.80") {
		t.Fatal("o contador do par foi apagado no sucesso")
	}
}

func TestTwoReplicasShareBlockState(t *testing.T) {
	kms := newKMS()
	mr := miniredis.RunT(t)
	a, b := newAuthEnv(t, mr, kms), newAuthEnv(t, mr, kms)
	const ip = "192.0.2.90:1"
	for i := 0; i < 3; i++ {
		a.bad(ip)
		b.bad(ip)
	}
	if a.good(ip).Code != 429 || b.good(ip).Code != 429 {
		t.Fatal("falhas somadas nas duas réplicas bloqueiam as duas")
	}
}

// Redis fora: continua respondendo, o bloqueio vale por réplica e a métrica de falha do armazenamento cresce.
func TestRedisDownKeepsServingWithLocalBlock(t *testing.T) {
	mr := miniredis.RunT(t)
	e := newAuthEnv(t, mr, newKMS())
	mr.Close()
	const ip = "192.0.2.100:1"
	if w := e.good(ip); w.Code != 200 {
		t.Fatalf("Redis fora: %d", w.Code)
	}
	for i := 0; i < 6; i++ {
		if w := e.bad(ip); w.Code != 401 {
			t.Fatalf("falha %d: %d", i, w.Code)
		}
	}
	if w := e.good(ip); w.Code != 429 {
		t.Fatalf("o bloqueio local vale: %d", w.Code)
	}
	if e.met.storeFailures == 0 {
		t.Fatal("tccelta_auth_block_store_failures_total deveria crescer")
	}
}

func TestWithoutRedisConfigured(t *testing.T) {
	e := newAuthEnv(t, nil, newKMS())
	for i := 0; i < 6; i++ {
		e.bad("192.0.2.110:1")
	}
	if e.good("192.0.2.110:1").Code != 429 || e.good("192.0.2.111:1").Code != 200 {
		t.Fatal("estado local por IP")
	}
}

func TestKMSUnavailableOnToken(t *testing.T) {
	kms := newKMS()
	e := newAuthEnv(t, miniredis.RunT(t), kms)
	f := error(port.ErrKMSUnavailable)
	kms.Failure.Store(&f)
	for i := 0; i < 8; i++ {
		w := e.good("192.0.2.120:1")
		b := oauthBody(t, w)
		if w.Code != 503 || b["error"] != "temporarily_unavailable" || b["codigo"] != "kms_indisponivel" || w.Header().Get("Retry-After") == "" {
			t.Fatalf("%d %v %s", w.Code, w.Header(), w.Body.String())
		}
		assertNoStore(t, w, "503")
	}
	var none error
	kms.Failure.Store(&none)
	if w := e.good("192.0.2.120:1"); w.Code != 200 {
		t.Fatalf("503 não conta como falha: %d", w.Code)
	}
	if e.met.count("kms_unavailable") != 8 {
		t.Fatalf("%v", e.met.results)
	}
}

type failingIssue struct{ err error }

func (f failingIssue) Run(context.Context, clientUC.Credentials) (clientUC.Token, error) {
	return clientUC.Token{}, f.err
}

func TestTokenInternalError(t *testing.T) {
	e := newAuthEnv(t, nil, newKMS())
	e.d.IssueToken = failingIssue{err: errors.New("boom")}
	w := reqFrom(e.d.Router(), "192.0.2.1:1", "POST", "/v1/auth/token", formOK, map[string]string{"Content-Type": formCT, "Authorization": basic(testClient, testSecret)})
	if b := oauthBody(t, w); w.Code != 500 || b["error"] != "server_error" || b["codigo"] != "erro_interno" {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	assertNoStore(t, w, "500")
}

func TestTokenRouteRateLimitedByIP(t *testing.T) {
	e := newAuthEnv(t, nil, newKMS())
	e.d.Cfg.Limits.IPMin = 2
	e.d.Limiter = ratelimit.NewLocal()
	h := e.d.Router()
	for i := 0; i < 2; i++ {
		reqFrom(h, "192.0.2.130:1", "GET", "/.well-known/jwks.json", "", nil)
	}
	w := reqFrom(h, "192.0.2.130:1", "POST", "/v1/auth/token", formOK, map[string]string{"Content-Type": formCT})
	if w.Code != 429 || code(w) != "limite_taxa" {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	assertNoStore(t, w, "limite de taxa")
}

func TestClientIP(t *testing.T) {
	d := &Deps{}
	d.Cfg.HTTP.TrustedProxies = []string{"172.30.10.0/24"}
	cases := []struct {
		name, remote, realIP, want string
	}{
		{"direto", "198.51.100.7:4000", "", "198.51.100.7"},
		{"proxy confiável com X-Real-IP", "172.30.10.5:4000", "203.0.113.9", "203.0.113.9"},
		{"X-Real-IP vindo de fora é ignorado", "198.51.100.7:4000", "203.0.113.9", "198.51.100.7"},
		{"X-Real-IP não é IP: usa o host", "172.30.10.5:4000", "isto-nao-e-ip", "172.30.10.5"},
		{"X-Real-IP vazio: usa o host", "172.30.10.5:4000", "", "172.30.10.5"},
		{"X-Real-IP com espaços", "172.30.10.5:4000", "  203.0.113.9 ", "203.0.113.9"},
		{"IPv4 mapeado em IPv6", "172.30.10.5:4000", "::ffff:203.0.113.9", "203.0.113.9"},
		{"IPv6 agrupado por /64", "172.30.10.5:4000", "2001:db8:1:2:aaaa:bbbb:cccc:dddd", "2001:db8:1:2::/64"},
		{"IPv6 do mesmo /64", "172.30.10.5:4000", "2001:db8:1:2:1111:2222:3333:4444", "2001:db8:1:2::/64"},
		{"IPv6 com zona", "172.30.10.5:4000", "fe80::1%eth0", "fe80::/64"},
		{"RemoteAddr IPv6 direto", "[2001:db8::7]:4000", "", "2001:db8::/64"},
		{"RemoteAddr sem porta", "198.51.100.7", "", "198.51.100.7"},
		{"RemoteAddr lixo", "???", "", "???"},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = c.remote
		if c.realIP != "" {
			r.Header.Set("X-Real-IP", c.realIP)
		}
		if got := d.clientIP(r); got != c.want {
			t.Errorf("%s: %q, esperava %q", c.name, got, c.want)
		}
	}
}

// IPv6 do mesmo /64 divide o contador; X-Real-IP forjado de fora dos proxies confiáveis não troca de contador.
func TestBlockCounterUsesNormalizedIP(t *testing.T) {
	e := newAuthEnv(t, miniredis.RunT(t), newKMS())
	hdr := func(ip string) map[string]string {
		return map[string]string{"Content-Type": formCT, "Authorization": basic(testClient, "errado"), "X-Real-IP": ip}
	}
	for i := 0; i < 6; i++ { // cada falha com um endereço diferente do mesmo /64, vindas do proxy confiável
		reqFrom(e.h, "172.30.10.5:1", "POST", "/v1/auth/token", formOK, hdr("2001:db8:aa:bb::"+strconv.Itoa(i+1)))
	}
	w := reqFrom(e.h, "172.30.10.5:1", "POST", "/v1/auth/token", formOK, map[string]string{"Content-Type": formCT,
		"Authorization": basic(testClient, testSecret), "X-Real-IP": "2001:db8:aa:bb:ffff::9"})
	if w.Code != 429 {
		t.Fatalf("o /64 divide o contador: %d", w.Code)
	}
	// de fora dos proxies, o cabeçalho é ignorado: forjar X-Real-IP não escapa do bloqueio do IP real
	for i := 0; i < 6; i++ {
		reqFrom(e.h, "198.51.100.1:1", "POST", "/v1/auth/token", formOK, hdr("203.0.113."+strconv.Itoa(i+1)))
	}
	w = reqFrom(e.h, "198.51.100.1:1", "POST", "/v1/auth/token", formOK, map[string]string{"Content-Type": formCT,
		"Authorization": basic(testClient, testSecret), "X-Real-IP": "203.0.113.200"})
	if w.Code != 429 {
		t.Fatalf("X-Real-IP forjado: %d", w.Code)
	}
	// X-Real-IP inválido vindo do proxy: cai no host do proxy
	for i := 0; i < 6; i++ {
		reqFrom(e.h, "172.30.10.6:1", "POST", "/v1/auth/token", formOK, hdr("lixo"))
	}
	if w := reqFrom(e.h, "172.30.10.6:2", "POST", "/v1/auth/token", formOK, map[string]string{"Content-Type": formCT,
		"Authorization": basic(testClient, testSecret), "X-Real-IP": "lixo"}); w.Code != 429 {
		t.Fatalf("X-Real-IP inválido: %d", w.Code)
	}
}

func TestJWKS(t *testing.T) {
	kms := newKMS()
	e := newAuthEnv(t, nil, kms)
	w := reqFrom(e.h, "192.0.2.1:1", "GET", "/.well-known/jwks.json", "", nil)
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/jwk-set+json" || w.Header().Get("Cache-Control") != "public, max-age=300" {
		t.Fatal(w.Code, w.Header())
	}
	var set struct {
		Keys []map[string]string `json:"keys"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &set); err != nil || len(set.Keys) != 1 {
		t.Fatalf("%v %s", err, w.Body.String())
	}
	k := set.Keys[0]
	pubs, _ := kms.ClientPublicKeys(context.Background())
	x, _ := base64.RawURLEncoding.DecodeString(k["x"])
	if k["kty"] != "OKP" || k["crv"] != "Ed25519" || k["alg"] != "EdDSA" || k["use"] != "sig" || k["kid"] != "cli-v1" || !bytes.Equal(x, pubs[1]) || len(x) != 32 {
		t.Fatalf("%v", k)
	}
	// só chaves cli: nunca a do app
	appPub, _ := kms.PublicKeys(context.Background())
	if strings.Contains(w.Body.String(), "jwt-v") || strings.Contains(w.Body.String(), base64.RawURLEncoding.EncodeToString(appPub[1])) {
		t.Fatal("o JWKS publica a chave do app")
	}
	// rotação: a chave nova aparece sem reiniciar (a recarga periódica do validador)
	kms.RotateCLI()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go e.val.UpdatePeriodically(ctx, 5*time.Millisecond)
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		w = reqFrom(e.h, "192.0.2.1:1", "GET", "/.well-known/jwks.json", "", nil)
		set.Keys = nil
		_ = json.Unmarshal(w.Body.Bytes(), &set)
		if len(set.Keys) == 2 {
			break
		}
	}
	if len(set.Keys) != 2 || set.Keys[1]["kid"] != "cli-v2" {
		t.Fatalf("%s", w.Body.String())
	}
}

type noKeys struct{}

func (noKeys) Run(context.Context) ([]port.ClientKey, error) { return nil, clientUC.ErrNoKeys }

func TestJWKSWithoutKeys(t *testing.T) {
	e := newAuthEnv(t, nil, newKMS())
	e.d.ClientKeys = noKeys{}
	w := reqFrom(e.d.Router(), "192.0.2.1:1", "GET", "/.well-known/jwks.json", "", nil)
	if w.Code != 503 || code(w) != "chaves_indisponiveis" || w.Header().Get("Retry-After") == "" {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestTripRoutesRequireBearer(t *testing.T) {
	kms := newKMS()
	e := newAuthEnv(t, nil, kms)
	e.create.out = trip.Trip{ID: tripID, Status: trip.StatusPending}
	id := "/v1/viagens/" + tripID.String()
	routes := [][2]string{{"POST", "/v1/viagens"}, {"GET", id}, {"GET", id + "/analise"}, {"GET", "/v1/viagens"}}
	call := func(authz string) []int {
		var out []int
		for _, r := range routes {
			hdr := map[string]string{"Content-Type": "application/json"}
			if authz != "" {
				hdr["Authorization"] = authz
			}
			out = append(out, req(e.h, r[0], r[1], []byte(validBody), hdr).Code)
		}
		return out
	}
	// token_ausente
	for _, authz := range []string{"", "Basic abc", "Bearer ", "Bearer"} {
		w := req(e.h, "GET", id, nil, map[string]string{"Authorization": authz})
		if w.Code != 401 || code(w) != "token_ausente" || w.Header().Get("WWW-Authenticate") != `Bearer realm="tccelta"` {
			t.Errorf("%q: %d %s %v", authz, w.Code, w.Body.String(), w.Header())
		}
	}
	// token_invalido
	w := req(e.h, "GET", id, nil, map[string]string{"Authorization": "Bearer lixo"})
	if w.Code != 401 || code(w) != "token_invalido" || w.Header().Get("WWW-Authenticate") != `Bearer error="invalid_token"` {
		t.Errorf("%d %s %v", w.Code, w.Body.String(), w.Header())
	}
	if got := call("Bearer lixo"); got[0] != 401 || got[1] != 401 || got[2] != 401 || got[3] != 401 {
		t.Fatal(got)
	}
	// token válido: 202 na criação e 200 nas leituras
	tok := oauthToken(t, e)
	e.get.out = trip.Trip{ID: tripID, Status: trip.StatusPending}
	if got := call("Bearer " + tok); got[0] != 202 || got[1] != 200 || got[3] != 200 {
		t.Fatalf("com token válido: %v", got)
	}
	// token_expirado: o validador anda 2 h no tempo
	e.val.SetClock(func() time.Time { return time.Now().Add(2 * time.Hour) })
	w = req(e.h, "GET", id, nil, map[string]string{"Authorization": "Bearer " + tok})
	if w.Code != 401 || code(w) != "token_expirado" || !strings.Contains(w.Header().Get("WWW-Authenticate"), "expirado") {
		t.Errorf("%d %s %v", w.Code, w.Body.String(), w.Header())
	}
}

func oauthToken(t *testing.T, e *authEnv) string {
	t.Helper()
	w := e.good("192.0.2.1:1")
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || out["access_token"] == nil {
		t.Fatalf("%v %s", err, w.Body.String())
	}
	return out["access_token"].(string)
}

type failingAuthClient struct{}

func (failingAuthClient) Run(context.Context, string) (port.ClientSession, error) {
	return port.ClientSession{}, errors.New("boom")
}

func TestAuthenticateClientInternalError(t *testing.T) {
	e := newAuthEnv(t, nil, newKMS())
	e.d.AuthenticateClient = failingAuthClient{}
	w := req(e.d.Router(), "GET", "/v1/viagens/"+tripID.String(), nil, map[string]string{"Authorization": "Bearer x"})
	if w.Code != 500 || code(w) != "erro_interno" {
		t.Fatal(w.Code, w.Body.String())
	}
}

// Um token do app é recusado nas rotas de viagem e um token de cliente é recusado na ingestão.
func TestAppAndClientTokensDoNotCross(t *testing.T) {
	kms := newKMS()
	e := newAuthEnv(t, nil, kms)
	ctx := context.Background()
	appIss := jwt.NewIssuer(kms, "iss", "tccelta-ingestao", time.Hour)
	appTok, err := appIss.Issue(ctx, devID(t), 3)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"/v1/viagens/" + tripID.String(), "/v1/viagens/" + tripID.String() + "/analise", "/v1/viagens"} {
		w := req(e.h, "GET", p, nil, map[string]string{"Authorization": "Bearer " + appTok.Token})
		if w.Code != 401 || code(w) != "token_invalido" {
			t.Errorf("token do app em %s: %d %s", p, w.Code, w.Body.String())
		}
	}
	if w := req(e.h, "POST", "/v1/viagens", []byte(validBody), map[string]string{"Content-Type": "application/json", "Authorization": "Bearer " + appTok.Token}); w.Code != 401 {
		t.Errorf("token do app na criação: %d", w.Code)
	}

	// token de cliente na ingestão: o mesmo roteador, com a autenticação do app de verdade
	f := newFakes()
	d, _ := build(t, f, true)
	tok := oauthToken(t, e)
	h := d.Router()
	for _, c := range [][2]string{{"POST", "/v1/dongles/blocos"}, {"GET", "/v1/dongles/0011223344556677/confirmacao"}} {
		w := req(h, c[0], c[1], []byte("x"), map[string]string{"Authorization": "Bearer " + tok})
		if w.Code != 401 || code(w) != "token_invalido" {
			t.Errorf("token de cliente em %s: %d %s", c[1], w.Code, w.Body.String())
		}
	}
}

func devID(t *testing.T) protocol.DevID {
	t.Helper()
	id, err := protocol.DevIDFromHex("0011223344556677")
	if err != nil {
		t.Fatal(err)
	}
	return id
}
