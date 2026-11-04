package simulator

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestParseClientCred(t *testing.T) {
	c, err := ParseClientCred("next-dev:/saida/segredo:ok")
	if err != nil || c.ID != "next-dev" || c.SecretFile != "/saida/segredo" || !c.Valid {
		t.Fatalf("%+v %v", c, err)
	}
	if c, err = ParseClientCred("velho:/x:invalido"); err != nil || c.Valid {
		t.Fatalf("%+v %v", c, err)
	}
	for _, bad := range []string{"", "a", "a:b", "a:b:talvez", ":b:ok", "a::ok", "a:b:ok:c"} {
		if _, err := ParseClientCred(bad); err == nil {
			t.Errorf("%q deveria falhar", bad)
		}
	}
}

func TestClientSecretFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s")
	_ = os.WriteFile(p, []byte("segredo\n"), 0o600)
	s, err := ClientCred{ID: "next-dev", SecretFile: p, Valid: true}.Secret()
	if err != nil || s != "segredo" {
		t.Fatalf("%q %v", s, err)
	}
	if _, err := (ClientCred{SecretFile: filepath.Join(t.TempDir(), "nao")}).Secret(); err == nil {
		t.Fatal("arquivo inexistente")
	}
	id, secret, err := primaryClient(Options{Clients: []ClientCred{{ID: "velho", SecretFile: p}, {ID: "next-dev", SecretFile: p, Valid: true}}})
	if err != nil || id != "next-dev" || secret != "segredo" {
		t.Fatalf("%q %q %v", id, secret, err)
	}
	if _, _, err := primaryClient(Options{Clients: []ClientCred{{ID: "velho", SecretFile: p}}}); err == nil {
		t.Fatal("sem cliente válido")
	}
}

func TestJWTPartsAndMetrics(t *testing.T) {
	// header {"alg":"EdDSA","kid":"cli-v1"}, claims {"sub":"x"}, assinatura "AAAA"
	h, c, signed, _, ok := jwtParts("eyJhbGciOiJFZERTQSIsImtpZCI6ImNsaS12MSJ9.eyJzdWIiOiJ4In0.AAAA")
	if !ok || jstr(h, "kid") != "cli-v1" || jstr(c, "sub") != "x" || signed == "" {
		t.Fatalf("%v %v %q", h, c, signed)
	}
	for _, bad := range []string{"", "a.b", "a.b.c", "!!.!!.!!"} {
		if _, _, _, _, ok := jwtParts(bad); ok {
			t.Errorf("%q", bad)
		}
	}
	lines := []string{`tccelta_auth_token_requests_total{result="issued"} 3`, `tccelta_auth_blocks_total{scope="pair"} 1`}
	if metricValue(lines, `tccelta_auth_token_requests_total{result="issued"}`) != 3 || metricValue(lines, "nao_existe") != 0 {
		t.Fatal("metricValue")
	}
}

func TestHTTPHelpers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/auth/token":
			if u, p, ok := r.BasicAuth(); !ok || u != "next-dev" || p != "ok" {
				w.WriteHeader(401)
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"t","token_type":"Bearer","expires_in":1800}`))
		case "/.well-known/jwks.json":
			_, _ = w.Write([]byte(`{"keys":[{"kty":"OKP","crv":"Ed25519","kid":"cli-v1","x":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"},{"kty":"RSA","kid":"x"}]}`))
		case "/metricas":
			_, _ = w.Write([]byte("# HELP x\ntccelta_auth_blocks_total{scope=\"ip\"} 2\noutra 1\n"))
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	if tok, err := fetchToken(ctx, srv.Client(), srv.URL, "next-dev", "ok"); err != nil || tok != "t" {
		t.Fatalf("%q %v", tok, err)
	}
	if _, err := fetchToken(ctx, srv.Client(), srv.URL, "next-dev", "errado"); err == nil {
		t.Fatal("401 deveria falhar")
	}
	_, keys, err := jwksKeys(ctx, srv.Client(), srv.URL)
	if err != nil || len(keys) != 1 || len(keys["cli-v1"]) != 32 {
		t.Fatalf("%v %v", keys, err)
	}
	if ls := metricLines(ctx, srv.Client(), srv.URL+"/metricas", "tccelta_auth_"); len(ls) != 1 || metricValue(ls, `tccelta_auth_blocks_total{scope="ip"}`) != 2 {
		t.Fatalf("%v", ls)
	}
	if retryAfter(http.Header{"Retry-After": {"7"}}) != 7 || retryAfter(http.Header{}) != 0 {
		t.Fatal("retryAfter")
	}
}
