package server

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tccelta/backend/internal/kms/config"
	"tccelta/backend/internal/kms/metrics"
	"tccelta/backend/internal/kms/provisioning"
	"tccelta/backend/internal/kms/vault"
	"tccelta/backend/internal/kms/vault/vaulttest"
)

var clientCfg = config.VaultConfig{MountAuth: "transit-auth", MountRec: "transit-rec", MountJWT: "transit-jwt", JWTKey: "jwt",
	MountClients: "kv-clientes", ClientJWTKey: "cli"}

// newClientServer monta o servidor com o cliente "next-dev" ativo (segredo devolvido) e o "velho-dev" revogado.
func newClientServer(t *testing.T) (*Server, *vaulttest.Fake, string, *metrics.M) {
	t.Helper()
	f := vaulttest.New()
	cl := &provisioning.Clients{T: f, Mount: clientCfg.MountClients}
	secret, err := cl.Create(t.Context(), "next-dev")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cl.Create(t.Context(), "velho-dev"); err != nil {
		t.Fatal(err)
	}
	if err := cl.Revoke(t.Context(), "velho-dev"); err != nil {
		t.Fatal(err)
	}
	m := metrics.New()
	v := &vaultReady{Vault: vault.OnTransport(f, clientCfg), ready: true}
	return &Server{Vault: v, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Metrics: m}, f, secret, m
}

func TestVerifyClientTable(t *testing.T) {
	s, f, secret, _ := newClientServer(t)
	h := s.Router()
	cases := []struct{ name, id, secret string }{
		{"segredo errado", "next-dev", strings.Repeat("a", 43)},
		{"cliente inexistente", "nao-existe", secret},
		{"cliente revogado", "velho-dev", secret},
	}
	if w := post(h, "/v1/clients/verify", map[string]any{"client_id": "next-dev", "secret": secret}); w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"valid":true}` {
		t.Fatalf("segredo certo: %d %s", w.Code, w.Body.String())
	}
	var first string
	for _, c := range cases {
		before := f.Calls["kv-clientes/data/clients/"+c.id]
		w := post(h, "/v1/clients/verify", map[string]any{"client_id": c.id, "secret": c.secret})
		if w.Code != 200 || w.Header().Get("Content-Type") != "application/json" {
			t.Fatalf("%s: %d %s", c.name, w.Code, w.Body.String())
		}
		// os três casos devolvem o mesmo corpo e executam a mesma leitura no Vault
		if first == "" {
			first = w.Body.String()
		}
		if w.Body.String() != first || strings.TrimSpace(first) != `{"valid":false}` {
			t.Errorf("%s: corpo %q difere de %q", c.name, w.Body.String(), first)
		}
		if f.Calls["kv-clientes/data/clients/"+c.id]-before != 1 {
			t.Errorf("%s: esperava uma leitura no Vault", c.name)
		}
	}
}

func TestVerifyClientValidation(t *testing.T) {
	s, f, _, _ := newClientServer(t)
	h := s.Router()
	before := len(f.Calls)
	for name, body := range map[string]map[string]any{
		"id com barra":   {"client_id": "../sys", "secret": "x"},
		"id curto":       {"client_id": "ab", "secret": "x"},
		"id maiúsculo":   {"client_id": "Next-Dev", "secret": "x"},
		"id começa em -": {"client_id": "-next", "secret": "x"},
		"segredo vazio":  {"client_id": "next-dev", "secret": ""},
		"segredo enorme": {"client_id": "next-dev", "secret": strings.Repeat("a", 129)},
		"sem id":         {"secret": "x"},
	} {
		w := post(h, "/v1/clients/verify", body)
		if w.Code != 400 || code(w) != "entradas_invalidas" {
			t.Errorf("%s: %d %s", name, w.Code, w.Body.String())
		}
	}
	if w := post(h, "/v1/clients/verify", map[string]any{"client_id": "next-dev", "secret": "x", "extra": 1}); w.Code != 400 || code(w) != "corpo_malformado" {
		t.Errorf("campo extra: %d", w.Code)
	}
	if len(f.Calls) != before {
		t.Fatalf("entrada inválida chegou ao Vault: %v", f.Calls)
	}
	// o limite de 128 caracteres vale; 128 chega ao Vault
	if w := post(h, "/v1/clients/verify", map[string]any{"client_id": "next-dev", "secret": strings.Repeat("a", 128)}); w.Code != 200 {
		t.Errorf("128 caracteres: %d", w.Code)
	}
}

func TestVerifyClientMetricAndNoSecretInLog(t *testing.T) {
	s, _, secret, m := newClientServer(t)
	var logs bytes.Buffer
	s.Log = slog.New(slog.NewTextHandler(&logs, nil))
	h := s.Router()
	post(h, "/v1/clients/verify", map[string]any{"client_id": "next-dev", "secret": secret})
	post(h, "/v1/clients/verify", map[string]any{"client_id": "next-dev", "secret": "errado"})
	if strings.Contains(logs.String(), secret) || strings.Contains(logs.String(), "errado") {
		t.Fatalf("o segredo apareceu no log: %s", logs.String())
	}
	for _, res := range []string{"valid", "invalid"} {
		if n := counter(t, m, "tccelta_kmssvc_client_verify_total", "result", res); n != 1 {
			t.Errorf("%s: %v", res, n)
		}
	}
}

func counter(t *testing.T, m *metrics.M, name, label, value string) float64 {
	t.Helper()
	fams, err := m.Reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range fams {
		if f.GetName() != name {
			continue
		}
		for _, mm := range f.GetMetric() {
			for _, l := range mm.GetLabel() {
				if l.GetName() == label && l.GetValue() == value {
					return mm.GetCounter().GetValue()
				}
			}
		}
	}
	return 0
}

func TestClientJWTSignAndKeys(t *testing.T) {
	s, f, _, _ := newClientServer(t)
	h := s.Router()
	w := post(h, "/v1/client-jwt/sign", map[string]any{"input": b("cabecalho.claims")})
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var a respSign
	_ = json.Unmarshal(w.Body.Bytes(), &a)
	sig, _ := base64.StdEncoding.DecodeString(a.Signature)
	if a.Version != 1 || len(sig) != 64 || !ed25519.Verify(f.PublicCLI(1), []byte("cabecalho.claims"), sig) {
		t.Fatalf("%+v", a)
	}
	// a assinatura da cli não vale com a chave do app
	if ed25519.Verify(f.PublicJWT(1), []byte("cabecalho.claims"), sig) {
		t.Fatal("a chave cli não pode ser a do app")
	}
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest("GET", "/v1/client-jwt/keys", nil))
	var ks map[string]string
	_ = json.Unmarshal(r.Body.Bytes(), &ks)
	pk, _ := base64.StdEncoding.DecodeString(ks["1"])
	if r.Code != 200 || len(ks) != 1 || !bytes.Equal(pk, f.PublicCLI(1)) {
		t.Fatal(r.Code, r.Body.String())
	}
	// a rota do app segue devolvendo só a chave jwt
	r = httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest("GET", "/v1/jwt/keys", nil))
	_ = json.Unmarshal(r.Body.Bytes(), &ks)
	pk, _ = base64.StdEncoding.DecodeString(ks["1"])
	if !bytes.Equal(pk, f.PublicJWT(1)) {
		t.Fatal("jwt/keys mudou")
	}
	for _, bad := range []map[string]any{{"input": ""}, {"input": "!!"}} {
		if w := post(h, "/v1/client-jwt/sign", bad); w.Code != 400 || code(w) != "entradas_invalidas" {
			t.Errorf("%v: %d", bad, w.Code)
		}
	}
}

func TestClientRoutesVaultUnavailable(t *testing.T) {
	s, f, _, _ := newClientServer(t)
	f.Failure = errors.New("vault fora do ar")
	h := s.Router()
	for _, c := range []struct {
		path string
		body any
	}{
		{"/v1/clients/verify", map[string]any{"client_id": "next-dev", "secret": "x"}},
		{"/v1/client-jwt/sign", map[string]any{"input": b("x")}},
	} {
		w := post(h, c.path, c.body)
		if w.Code != 503 || code(w) != "kms_indisponivel" || w.Header().Get("Retry-After") == "" {
			t.Errorf("%s: %d %s", c.path, w.Code, w.Body.String())
		}
	}
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest("GET", "/v1/client-jwt/keys", nil))
	if r.Code != 503 {
		t.Fatal(r.Code)
	}
}

func TestClientRoutesRequireClientCert(t *testing.T) {
	s, _, _, _ := newClientServer(t)
	s.RequireClientCert = true
	h := s.Router()
	for _, p := range []string{"/v1/clients/verify", "/v1/client-jwt/sign"} {
		if w := post(h, p, map[string]any{}); w.Code != http.StatusUnauthorized {
			t.Errorf("%s sem certificado: %d", p, w.Code)
		}
	}
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest("GET", "/v1/client-jwt/keys", nil))
	if r.Code != http.StatusUnauthorized {
		t.Fatal(r.Code)
	}
}
