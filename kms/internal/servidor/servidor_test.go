package servidor

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

	"tccelta/kms/internal/config"
	"tccelta/kms/internal/vault"
	"tccelta/kms/internal/vault/vaultteste"
)

var (
	kAuth = []byte("0123456789abcdef0123456789abcdef")
	kRec  = []byte("fedcba9876543210fedcba9876543210")
)

// vaultPronto troca o Pronto() do Vault de teste (que não tem cliente de baixo nível).
type vaultPronto struct {
	*vault.Vault
	pronto bool
}

func (v vaultPronto) Pronto() bool { return v.pronto }

func novo(t *testing.T) (*Servidor, *vaultteste.Falso, *vaultPronto) {
	t.Helper()
	f := vaultteste.Novo()
	f.AdicionarDongle("0011223344556677", kAuth, kRec)
	cfg := config.VaultConfig{MountAuth: "transit-auth", MountRec: "transit-rec", MountJWT: "transit-jwt", ChaveJWT: "jwt"}
	v := &vaultPronto{Vault: vault.SobreTransporte(f, cfg), pronto: true}
	return &Servidor{Vault: v, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}, f, v
}

func post(h http.Handler, caminho string, corpo any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(corpo)
	r := httptest.NewRequest("POST", caminho, bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func codigo(w *httptest.ResponseRecorder) string {
	var m map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	c, _ := m["codigo"].(string)
	return c
}

func macLocal(k, msg []byte) []byte {
	h := hmac.New(sha256.New, k)
	h.Write(msg)
	return h.Sum(nil)
}

func b(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func TestHMAC(t *testing.T) {
	s, _, _ := novo(t)
	w := post(s.Roteador(), "/v1/hmac", map[string]any{"chave": "0011223344556677", "entradas": []string{b("um"), b("dois")}})
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
		if !bytes.Equal(got, macLocal(kAuth, []byte(in))) || len(got) != 32 {
			t.Errorf("mac %d não confere", i)
		}
	}
}

func TestVerificarBlocos(t *testing.T) {
	s, _, _ := novo(t)
	boa := macLocal(kRec, []byte("bloco"))
	ruim := bytes.Clone(boa)
	ruim[0] ^= 1
	w := post(s.Roteador(), "/v1/blocos/verificar", map[string]any{"chave": "0011223344556677",
		"entradas": []string{b("bloco"), b("bloco")}, "tags": []string{base64.StdEncoding.EncodeToString(boa), base64.StdEncoding.EncodeToString(ruim)}})
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var r respVerificar
	_ = json.Unmarshal(w.Body.Bytes(), &r)
	if len(r.Validos) != 2 || !r.Validos[0] || r.Validos[1] {
		t.Fatalf("%+v", r)
	}
}

func TestJWT(t *testing.T) {
	s, f, _ := novo(t)
	h := s.Roteador()
	w := post(h, "/v1/jwt/assinar", map[string]any{"entrada": b("cabecalho.claims")})
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var a respAssinar
	_ = json.Unmarshal(w.Body.Bytes(), &a)
	sig, _ := base64.StdEncoding.DecodeString(a.Assinatura)
	if a.Versao != 1 || !ed25519.Verify(f.PublicaJWT(1), []byte("cabecalho.claims"), sig) {
		t.Fatalf("%+v", a)
	}
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest("GET", "/v1/jwt/chaves", nil))
	var ks map[string]string
	_ = json.Unmarshal(r.Body.Bytes(), &ks)
	pk, _ := base64.StdEncoding.DecodeString(ks["1"])
	if r.Code != 200 || !bytes.Equal(pk, f.PublicaJWT(1)) {
		t.Fatal(r.Code, r.Body.String())
	}
}

func TestValidacao(t *testing.T) {
	s, f, _ := novo(t)
	h := s.Roteador()
	casos := []struct {
		nome, caminho string
		corpo         any
		status        int
		cod           string
	}{
		{"chave com barra", "/v1/hmac", map[string]any{"chave": "../sys/x", "entradas": []string{b("a")}}, 400, "chave_invalida"},
		{"chave vazia", "/v1/hmac", map[string]any{"chave": "", "entradas": []string{b("a")}}, 400, "chave_invalida"},
		{"sem entradas", "/v1/hmac", map[string]any{"chave": "0011223344556677", "entradas": []string{}}, 400, "entradas_invalidas"},
		{"base64 sem padding", "/v1/hmac", map[string]any{"chave": "0011223344556677", "entradas": []string{"YQ"}}, 400, "entradas_invalidas"},
		{"campo extra", "/v1/hmac", map[string]any{"chave": "0011223344556677", "entradas": []string{b("a")}, "x": 1}, 400, "corpo_malformado"},
		{"tags a menos", "/v1/blocos/verificar", map[string]any{"chave": "0011223344556677", "entradas": []string{b("a"), b("b")}, "tags": []string{b("t")}}, 400, "entradas_invalidas"},
		{"assinar vazio", "/v1/jwt/assinar", map[string]any{"entrada": ""}, 400, "entradas_invalidas"},
	}
	for _, c := range casos {
		w := post(h, c.caminho, c.corpo)
		if w.Code != c.status || codigo(w) != c.cod || w.Header().Get("Content-Type") != "application/problem+json" {
			t.Errorf("%s: %d %s", c.nome, w.Code, w.Body.String())
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
	if len(f.Chamadas) != 0 {
		t.Fatalf("entradas inválidas não podem chegar ao Vault: %v", f.Chamadas)
	}
}

func TestVaultIndisponivelDevolve503ComRetryAfter(t *testing.T) {
	s, f, _ := novo(t)
	f.Falha = errors.New("vault fora do ar")
	h := s.Roteador()
	for _, c := range []struct {
		caminho string
		corpo   any
	}{
		{"/v1/hmac", map[string]any{"chave": "0011223344556677", "entradas": []string{b("a")}}},
		{"/v1/blocos/verificar", map[string]any{"chave": "0011223344556677", "entradas": []string{b("a")}, "tags": []string{b("t")}}},
		{"/v1/jwt/assinar", map[string]any{"entrada": b("x")}},
	} {
		w := post(h, c.caminho, c.corpo)
		if w.Code != 503 || codigo(w) != "kms_indisponivel" || w.Header().Get("Retry-After") == "" {
			t.Errorf("%s: %d %s %v", c.caminho, w.Code, w.Body.String(), w.Header())
		}
	}
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest("GET", "/v1/jwt/chaves", nil))
	if r.Code != 503 {
		t.Fatal(r.Code)
	}
}

func TestChaveDesconhecidaViraIndisponivel(t *testing.T) {
	// mesmo comportamento da API antiga: qualquer erro do Vault, inclusive chave inexistente, é 503
	s, _, _ := novo(t)
	w := post(s.Roteador(), "/v1/hmac", map[string]any{"chave": "ffffffffffffffff", "entradas": []string{b("a")}})
	if w.Code != 503 || codigo(w) != "kms_indisponivel" {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestSaude(t *testing.T) {
	s, _, v := novo(t)
	h := s.Roteador()
	get := func(p string) int {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", p, nil))
		return w.Code
	}
	if get("/saude/viva") != 200 || get("/saude/pronta") != 200 {
		t.Fatal("deveria estar pronto")
	}
	v.pronto = false
	if get("/saude/viva") != 200 || get("/saude/pronta") != 503 {
		t.Fatal("deveria estar não pronto")
	}
}

func TestExigirCertClienteSemTLS(t *testing.T) {
	s, _, _ := novo(t)
	s.ExigirCertCliente = true
	w := post(s.Roteador(), "/v1/hmac", map[string]any{"chave": "0011223344556677", "entradas": []string{b("a")}})
	if w.Code != 401 || codigo(w) != "cliente_nao_autenticado" {
		t.Fatal(w.Code, w.Body.String())
	}
}

// ---- mTLS de verdade ----

type pki struct {
	dir     string
	ca      *x509.Certificate
	caKey   *ecdsa.PrivateKey
	caPEM   []byte
	proximo int64
}

func novaPKI(t *testing.T) *pki {
	t.Helper()
	k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	modelo := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "ca-teste"}, NotBefore: time.Now().Add(-time.Hour),
		NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, modelo, modelo, &k.PublicKey, k)
	if err != nil {
		t.Fatal(err)
	}
	ca, _ := x509.ParseCertificate(der)
	return &pki{dir: t.TempDir(), ca: ca, caKey: k, caPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), proximo: 2}
}

// emitir cria cert+chave (PEM em arquivos) assinados pela CA; servidor=true põe SAN 127.0.0.1.
func (p *pki) emitir(t *testing.T, nome string, servidor bool) (certArq, chaveArq string) {
	t.Helper()
	k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	p.proximo++
	m := &x509.Certificate{SerialNumber: big.NewInt(p.proximo), Subject: pkix.Name{CommonName: nome}, NotBefore: time.Now().Add(-time.Hour),
		NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature}
	if servidor {
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
	certArq, chaveArq = filepath.Join(p.dir, nome+".pem"), filepath.Join(p.dir, nome+"-key.pem")
	_ = os.WriteFile(certArq, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600)
	_ = os.WriteFile(chaveArq, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kb}), 0o600)
	return
}

func TestMTLS(t *testing.T) {
	p := novaPKI(t)
	caArq := filepath.Join(p.dir, "ca.pem")
	_ = os.WriteFile(caArq, p.caPEM, 0o600)
	sc, sk := p.emitir(t, "kms", true)
	cc, ck := p.emitir(t, "api", false)
	tlsCfg, err := ConfigTLS(sc, sk, caArq)
	if err != nil {
		t.Fatal(err)
	}
	s, _, _ := novo(t)
	s.ExigirCertCliente = true
	srv := httptest.NewUnstartedServer(s.Roteador())
	srv.TLS = tlsCfg
	srv.StartTLS()
	defer srv.Close()

	raiz := x509.NewCertPool()
	raiz.AppendCertsFromPEM(p.caPEM)
	cliente := func(certs ...tls.Certificate) *http.Client {
		return &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: raiz, Certificates: certs}}}
	}
	corpo, _ := json.Marshal(map[string]any{"chave": "0011223344556677", "entradas": []string{b("a")}})

	par, err := tls.LoadX509KeyPair(cc, ck)
	if err != nil {
		t.Fatal(err)
	}
	res, err := cliente(par).Post(srv.URL+"/v1/hmac", "application/json", bytes.NewReader(corpo))
	if err != nil {
		t.Fatal("cliente com certificado deveria passar:", err)
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatal(res.StatusCode)
	}

	// sem certificado de cliente: a conexão é recusada
	if res, err := cliente().Post(srv.URL+"/v1/hmac", "application/json", bytes.NewReader(corpo)); err == nil {
		res.Body.Close()
		t.Fatalf("cliente sem certificado deveria ser recusado (status %d)", res.StatusCode)
	}

	// certificado de outra CA: também recusado
	outra := novaPKI(t)
	oc, ok := outra.emitir(t, "intruso", false)
	parRuim, _ := tls.LoadX509KeyPair(oc, ok)
	if res, err := cliente(parRuim).Post(srv.URL+"/v1/hmac", "application/json", bytes.NewReader(corpo)); err == nil {
		res.Body.Close()
		t.Fatalf("certificado de outra CA deveria ser recusado (status %d)", res.StatusCode)
	}
}
