package server

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tccelta/backend/internal/kms/config"
	"tccelta/backend/internal/kms/vault"
	"tccelta/backend/internal/kms/vault/vaulttest"
)

var (
	kAuth = []byte("0123456789abcdef0123456789abcdef")
	kRec  = []byte("fedcba9876543210fedcba9876543210")
)

// vaultReady troca o Pronto() do Vault de teste (que não tem cliente de baixo nível).
type vaultReady struct {
	*vault.Vault
	ready bool
}

func (v vaultReady) Ready() bool { return v.ready }

func newServer(t *testing.T) (*Server, *vaulttest.Fake, *vaultReady) {
	t.Helper()
	f := vaulttest.New()
	f.AddDongle("0011223344556677", kAuth, kRec)
	cfg := config.VaultConfig{MountAuth: "transit-auth", MountRec: "transit-rec", MountJWT: "transit-jwt", JWTKey: "jwt"}
	v := &vaultReady{Vault: vault.OnTransport(f, cfg), ready: true}
	return &Server{Vault: v, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}, f, v
}

func post(h http.Handler, path string, body any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	r := httptest.NewRequest("POST", path, bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func code(w *httptest.ResponseRecorder) string {
	var m map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	c, _ := m["codigo"].(string)
	return c
}

func localMAC(k, msg []byte) []byte {
	h := hmac.New(sha256.New, k)
	h.Write(msg)
	return h.Sum(nil)
}

func b(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func TestHMAC(t *testing.T) {
	s, _, _ := newServer(t)
	w := post(s.Router(), "/v1/hmac", map[string]any{"key": "0011223344556677", "inputs": []string{b("um"), b("dois")}})
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/json" {
		t.Fatal(w.Code, w.Body.String())
	}
	var r respHMAC
	_ = json.Unmarshal(w.Body.Bytes(), &r)
	if len(r.MACs) != 2 {
		t.Fatalf("%+v", r)
	}
	for i, in := range []string{"um", "dois"} {
		got, _ := base64.StdEncoding.DecodeString(r.MACs[i])
		if !bytes.Equal(got, localMAC(kAuth, []byte(in))) || len(got) != 32 {
			t.Errorf("mac %d não confere", i)
		}
	}
}

func TestVerifyBlocks(t *testing.T) {
	s, _, _ := newServer(t)
	good := localMAC(kRec, []byte("bloco"))
	bad := bytes.Clone(good)
	bad[0] ^= 1
	w := post(s.Router(), "/v1/blocks/verify", map[string]any{"key": "0011223344556677",
		"inputs": []string{b("bloco"), b("bloco")}, "tags": []string{base64.StdEncoding.EncodeToString(good), base64.StdEncoding.EncodeToString(bad)}})
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var r respVerify
	_ = json.Unmarshal(w.Body.Bytes(), &r)
	if len(r.Valid) != 2 || !r.Valid[0] || r.Valid[1] {
		t.Fatalf("%+v", r)
	}
}

func TestJWT(t *testing.T) {
	s, f, _ := newServer(t)
	h := s.Router()
	w := post(h, "/v1/jwt/sign", map[string]any{"input": b("cabecalho.claims")})
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var a respSign
	_ = json.Unmarshal(w.Body.Bytes(), &a)
	sig, _ := base64.StdEncoding.DecodeString(a.Signature)
	if a.Version != 1 || !ed25519.Verify(f.PublicJWT(1), []byte("cabecalho.claims"), sig) {
		t.Fatalf("%+v", a)
	}
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest("GET", "/v1/jwt/keys", nil))
	var ks map[string]string
	_ = json.Unmarshal(r.Body.Bytes(), &ks)
	pk, _ := base64.StdEncoding.DecodeString(ks["1"])
	if r.Code != 200 || !bytes.Equal(pk, f.PublicJWT(1)) {
		t.Fatal(r.Code, r.Body.String())
	}
}

func TestValidation(t *testing.T) {
	s, f, _ := newServer(t)
	h := s.Router()
	cases := []struct {
		name, path string
		body       any
		status     int
		code       string
	}{
		{"chave com barra", "/v1/hmac", map[string]any{"key": "../sys/x", "inputs": []string{b("a")}}, 400, "chave_invalida"},
		{"chave vazia", "/v1/hmac", map[string]any{"key": "", "inputs": []string{b("a")}}, 400, "chave_invalida"},
		{"sem entradas", "/v1/hmac", map[string]any{"key": "0011223344556677", "inputs": []string{}}, 400, "entradas_invalidas"},
		{"base64 sem padding", "/v1/hmac", map[string]any{"key": "0011223344556677", "inputs": []string{"YQ"}}, 400, "entradas_invalidas"},
		{"campo extra", "/v1/hmac", map[string]any{"key": "0011223344556677", "inputs": []string{b("a")}, "x": 1}, 400, "corpo_malformado"},
		{"tags a menos", "/v1/blocks/verify", map[string]any{"key": "0011223344556677", "inputs": []string{b("a"), b("b")}, "tags": []string{b("t")}}, 400, "entradas_invalidas"},
		{"assinar vazio", "/v1/jwt/sign", map[string]any{"input": ""}, 400, "entradas_invalidas"},
	}
	for _, c := range cases {
		w := post(h, c.path, c.body)
		if w.Code != c.status || code(w) != c.code || w.Header().Get("Content-Type") != "application/problem+json" {
			t.Errorf("%s: %d %s", c.name, w.Code, w.Body.String())
		}
	}
	// Content-Type errado
	r := httptest.NewRequest("POST", "/v1/hmac", strings.NewReader("{}"))
	r.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 415 {
		t.Fatal(w.Code)
	}
	// nada disso chegou ao Vault
	if len(f.Calls) != 0 {
		t.Fatalf("entradas inválidas não podem chegar ao Vault: %v", f.Calls)
	}
}

func TestVaultUnavailableReturns503WithRetryAfter(t *testing.T) {
	s, f, _ := newServer(t)
	f.Failure = errors.New("vault fora do ar")
	h := s.Router()
	for _, c := range []struct {
		path string
		body any
	}{
		{"/v1/hmac", map[string]any{"key": "0011223344556677", "inputs": []string{b("a")}}},
		{"/v1/blocks/verify", map[string]any{"key": "0011223344556677", "inputs": []string{b("a")}, "tags": []string{b("t")}}},
		{"/v1/jwt/sign", map[string]any{"input": b("x")}},
	} {
		w := post(h, c.path, c.body)
		if w.Code != 503 || code(w) != "kms_indisponivel" || w.Header().Get("Retry-After") == "" {
			t.Errorf("%s: %d %s %v", c.path, w.Code, w.Body.String(), w.Header())
		}
	}
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest("GET", "/v1/jwt/keys", nil))
	if r.Code != 503 {
		t.Fatal(r.Code)
	}
}

func TestUnknownKeyBecomesUnavailable(t *testing.T) {
	// mesmo comportamento da API antiga: qualquer erro do Vault, inclusive chave inexistente, é 503
	s, _, _ := newServer(t)
	w := post(s.Router(), "/v1/hmac", map[string]any{"key": "ffffffffffffffff", "inputs": []string{b("a")}})
	if w.Code != 503 || code(w) != "kms_indisponivel" {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestHealth(t *testing.T) {
	s, _, v := newServer(t)
	h := s.Router()
	get := func(p string) int {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", p, nil))
		return w.Code
	}
	if get("/saude/viva") != 200 || get("/saude/pronta") != 200 {
		t.Fatal("deveria estar pronto")
	}
	v.ready = false
	if get("/saude/viva") != 200 || get("/saude/pronta") != 503 {
		t.Fatal("deveria estar não pronto")
	}
}

func TestRequireClientCertWithoutTLS(t *testing.T) {
	s, _, _ := newServer(t)
	s.RequireClientCert = true
	s.ClientCN = "api"
	w := post(s.Router(), "/v1/hmac", map[string]any{"key": "0011223344556677", "inputs": []string{b("a")}})
	if w.Code != 401 || code(w) != "cliente_nao_autenticado" {
		t.Fatal(w.Code, w.Body.String())
	}
}

// ---- mTLS de verdade ----

type pki struct {
	dir   string
	ca    *x509.Certificate
	caKey *ecdsa.PrivateKey
	caPEM []byte
	next  int64
}

func newPKI(t *testing.T) *pki {
	t.Helper()
	k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "ca-teste"}, NotBefore: time.Now().Add(-time.Hour),
		NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &k.PublicKey, k)
	if err != nil {
		t.Fatal(err)
	}
	ca, _ := x509.ParseCertificate(der)
	return &pki{dir: t.TempDir(), ca: ca, caKey: k, caPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), next: 2}
}

// issue cria cert+chave (PEM em arquivos) assinados pela CA; servidor=true põe SAN 127.0.0.1.
func (p *pki) issue(t *testing.T, name string, server bool) (certFile, keyFile string) {
	t.Helper()
	k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	p.next++
	m := &x509.Certificate{SerialNumber: big.NewInt(p.next), Subject: pkix.Name{CommonName: name}, NotBefore: time.Now().Add(-time.Hour),
		NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature}
	if server {
		m.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		m.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
	} else {
		m.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	}
	der, err := x509.CreateCertificate(rand.Reader, m, p.ca, &k.PublicKey, p.caKey)
	if err != nil {
		t.Fatal(err)
	}
	kb, _ := x509.MarshalECPrivateKey(k)
	certFile, keyFile = filepath.Join(p.dir, name+".pem"), filepath.Join(p.dir, name+"-key.pem")
	_ = os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600)
	_ = os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kb}), 0o600)
	return
}

func TestMTLS(t *testing.T) {
	p := newPKI(t)
	caFile := filepath.Join(p.dir, "ca.pem")
	_ = os.WriteFile(caFile, p.caPEM, 0o600)
	sc, sk := p.issue(t, "kms", true)
	cc, ck := p.issue(t, "api", false)
	tlsCfg, err := ConfigTLS(sc, sk, caFile)
	if err != nil {
		t.Fatal(err)
	}
	s, _, _ := newServer(t)
	s.RequireClientCert = true
	s.ClientCN = "api"
	srv := httptest.NewUnstartedServer(s.Router())
	srv.TLS = tlsCfg
	srv.StartTLS()
	defer srv.Close()

	root := x509.NewCertPool()
	root.AppendCertsFromPEM(p.caPEM)
	client := func(certs ...tls.Certificate) *http.Client {
		return &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: root, Certificates: certs}}}
	}
	body, _ := json.Marshal(map[string]any{"key": "0011223344556677", "inputs": []string{b("a")}})

	pair, err := tls.LoadX509KeyPair(cc, ck)
	if err != nil {
		t.Fatal(err)
	}
	res, err := client(pair).Post(srv.URL+"/v1/hmac", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal("cliente com certificado deveria passar:", err)
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatal(res.StatusCode)
	}

	// mesma CA, outro CN: autenticado, mas não autorizado
	oc2, ok2 := p.issue(t, "operador", false)
	otherCNPair, err := tls.LoadX509KeyPair(oc2, ok2)
	if err != nil {
		t.Fatal(err)
	}
	res, err = client(otherCNPair).Post(srv.URL+"/v1/hmac", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 403 {
		t.Fatal("CN diferente deveria dar 403, veio", res.StatusCode)
	}

	// sem certificado de cliente: a conexão é recusada
	if res, err := client().Post(srv.URL+"/v1/hmac", "application/json", bytes.NewReader(body)); err == nil {
		res.Body.Close()
		t.Fatalf("cliente sem certificado deveria ser recusado (status %d)", res.StatusCode)
	}

	// certificado de outra CA: também recusado
	other := newPKI(t)
	oc, ok := other.issue(t, "intruso", false)
	badPair, _ := tls.LoadX509KeyPair(oc, ok)
	if res, err := client(badPair).Post(srv.URL+"/v1/hmac", "application/json", bytes.NewReader(body)); err == nil {
		res.Body.Close()
		t.Fatalf("certificado de outra CA deveria ser recusado (status %d)", res.StatusCode)
	}
}
