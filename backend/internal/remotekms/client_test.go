package remotekms

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"tccelta/backend/internal/observability"
	"tccelta/backend/internal/usecase/port"
	"tccelta/backend/internal/usecase/port/porttest"
)

// Os exemplos abaixo são os de kms/docs/contrato-http.md: se o contrato mudar, os dois lados mudam.
const (
	reqHMACExample = `{"key":"0011223344556677","inputs":["YQ==","Yg=="]}`
	// os dois MACs são 32 bytes: 0x01 repetido e 0x02 repetido
	respHMACExample = `{"macs":["AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE=","AgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgI="]}`

	reqVerifyExample  = `{"key":"0011223344556677","inputs":["YQ==","Yg=="],"tags":["dA==","dQ=="]}`
	respVerifyExample = `{"valid":[true,false]}`

	reqSignExample = `{"input":"YWJj"}`
)

func newTest(t *testing.T, h http.HandlerFunc, adjust ...func(*Options)) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	o := Options{URL: srv.URL, Timeout: 300 * time.Millisecond, MaxConcurrency: 4}
	for _, f := range adjust {
		f(&o)
	}
	c, err := New(o, porttest.Nop{})
	if err != nil {
		t.Fatal(err)
	}
	return c, srv
}

func bodyOf(t *testing.T, r *http.Request) string {
	t.Helper()
	b, _ := io.ReadAll(r.Body)
	return string(b)
}

func TestHMACAuth(t *testing.T) {
	c, _ := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/v1/hmac" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("%s %s %s", r.Method, r.URL.Path, r.Header.Get("Content-Type"))
		}
		if got := bodyOf(t, r); got != reqHMACExample {
			t.Errorf("corpo difere do contrato: %s", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, respHMACExample)
	})
	macs, err := c.HMACAuth(context.Background(), "0011223344556677", [][]byte{[]byte("a"), []byte("b")})
	if err != nil {
		t.Fatal(err)
	}
	if len(macs) != 2 || !bytes.Equal(macs[0], bytes.Repeat([]byte{1}, 32)) || !bytes.Equal(macs[1], bytes.Repeat([]byte{2}, 32)) {
		t.Fatalf("%x", macs)
	}
}

func TestVerifyBlocks(t *testing.T) {
	c, _ := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/blocks/verify" {
			t.Errorf("caminho %s", r.URL.Path)
		}
		if got := bodyOf(t, r); got != reqVerifyExample {
			t.Errorf("corpo difere do contrato: %s", got)
		}
		_, _ = io.WriteString(w, respVerifyExample)
	})
	ok, err := c.VerifyBlocks(context.Background(), "0011223344556677", [][]byte{[]byte("a"), []byte("b")}, [][]byte{[]byte("t"), []byte("u")})
	if err != nil || len(ok) != 2 || !ok[0] || ok[1] {
		t.Fatalf("%v %v", err, ok)
	}
	if _, err := c.VerifyBlocks(context.Background(), "x", [][]byte{[]byte("a")}, nil); err == nil {
		t.Fatal("tamanhos diferentes")
	}
}

func TestSignAndKeys(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	c, _ := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/jwt/sign":
			if got := bodyOf(t, r); got != reqSignExample {
				t.Errorf("corpo difere do contrato: %s", got)
			}
			sig := ed25519.Sign(priv, []byte("abc"))
			_ = json.NewEncoder(w).Encode(map[string]any{"signature": base64.StdEncoding.EncodeToString(sig), "version": 3})
		case "/v1/jwt/keys":
			if r.Method != "GET" {
				t.Errorf("método %s", r.Method)
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"2": base64.StdEncoding.EncodeToString(pub), "3": base64.StdEncoding.EncodeToString(pub)})
		default:
			t.Errorf("caminho %s", r.URL.Path)
		}
	})
	sig, verify, err := c.Sign(context.Background(), []byte("abc"))
	if err != nil || verify != 3 || !ed25519.Verify(pub, []byte("abc"), sig) {
		t.Fatalf("%v %d", err, verify)
	}
	ks, err := c.PublicKeys(context.Background())
	if err != nil || len(ks) != 2 || !bytes.Equal(ks[3], pub) {
		t.Fatalf("%v %v", err, ks)
	}
}

func TestInvalidResponseBecomesUnavailable(t *testing.T) {
	c, _ := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"macs":["AQ=="]}`) // 1 byte, e 1 item para 2 entradas
	})
	if _, err := c.HMACAuth(context.Background(), "k", [][]byte{[]byte("a"), []byte("b")}); !errors.Is(err, port.ErrKMSUnavailable) {
		t.Fatal(err)
	}
}

func TestFailuresBecomeUnavailable(t *testing.T) {
	problem503 := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.Header().Set("Retry-After", "2")
		w.WriteHeader(503)
		_, _ = io.WriteString(w, `{"codigo":"kms_indisponivel","detail":"Vault indisponível"}`)
	}
	usecases := map[string]http.HandlerFunc{
		"503":     problem503,
		"429":     func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(429) },
		"500":     func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) },
		"timeout": func(w http.ResponseWriter, r *http.Request) { time.Sleep(time.Second) },
		"json":    func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "isto não é JSON") },
	}
	for name, h := range usecases {
		c, _ := newTest(t, h)
		_, err := c.HMACAuth(context.Background(), "k", [][]byte{[]byte("a")})
		if !errors.Is(err, port.ErrKMSUnavailable) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestConnectionRefused(t *testing.T) {
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	url := "http://" + l.Addr().String()
	_ = l.Close() // ninguém escutando
	c, err := New(Options{URL: url, Timeout: 200 * time.Millisecond}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.HMACAuth(context.Background(), "k", [][]byte{[]byte("a")}); !errors.Is(err, port.ErrKMSUnavailable) {
		t.Fatal(err)
	}
	if c.Ready(context.Background()) {
		t.Fatal("sem servidor não pode estar pronto")
	}
}

func TestContractErrorIsNotUnavailability(t *testing.T) {
	var n atomic.Int32
	c, _ := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		w.WriteHeader(400)
		_, _ = io.WriteString(w, `{"codigo":"chave_invalida","detail":"nome de chave inválido"}`)
	})
	_, err := c.HMACAuth(context.Background(), "k", [][]byte{[]byte("a")})
	if err == nil || errors.Is(err, port.ErrKMSUnavailable) {
		t.Fatalf("4xx não é indisponibilidade: %v", err)
	}
	// e não conta para o disjuntor
	for i := 0; i < 10; i++ {
		_, _ = c.HMACAuth(context.Background(), "k", [][]byte{[]byte("a")})
	}
	if n.Load() != 11 {
		t.Fatalf("o disjuntor não deveria abrir com 4xx: %d chamadas", n.Load())
	}
}

func TestBreakerOpens(t *testing.T) {
	var n atomic.Int32
	c, _ := newTest(t, func(w http.ResponseWriter, r *http.Request) { n.Add(1); w.WriteHeader(503) })
	for i := 0; i < 5; i++ {
		if _, err := c.HMACAuth(context.Background(), "k", [][]byte{[]byte("a")}); !errors.Is(err, port.ErrKMSUnavailable) {
			t.Fatal(err)
		}
	}
	before := n.Load()
	_, err := c.HMACAuth(context.Background(), "k", [][]byte{[]byte("a")})
	if !errors.Is(err, port.ErrKMSUnavailable) || n.Load() != before {
		t.Fatalf("com o disjuntor aberto a chamada nem chega ao servidor: %v (%d → %d)", err, before, n.Load())
	}
	if c.Ready(context.Background()) {
		t.Fatal("disjuntor aberto => não pronto")
	}
}

func TestReady(t *testing.T) {
	var ok atomic.Bool
	ok.Store(true)
	c, _ := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/saude/pronta" {
			t.Errorf("caminho %s", r.URL.Path)
		}
		if ok.Load() {
			w.WriteHeader(200)
			return
		}
		w.WriteHeader(503)
	})
	if !c.Ready(context.Background()) {
		t.Fatal("deveria estar pronto")
	}
	ok.Store(false)
	c.mu.Lock()
	c.probeAt = time.Time{} // expira o cache de 2 s
	c.mu.Unlock()
	if c.Ready(context.Background()) {
		t.Fatal("KMS 503 => não pronto")
	}
}

func TestMetricsAndPhase(t *testing.T) {
	var (
		ops    []string
		phases []string
		closed bool
	)
	m := &metricsSpy{f: func(op, res, phase string) { ops, phases = append(ops, op+"/"+res), append(phases, phase) }, d: func(b bool) { closed = !b }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, respHMACExample) }))
	defer srv.Close()
	c, _ := New(Options{URL: srv.URL}, m)
	ctx := observability.WithKMSPhase(context.Background(), observability.PhaseTx)
	if _, err := c.HMACAuth(ctx, "k", [][]byte{[]byte("a"), []byte("b")}); err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 || ops[0] != "hmac_auth/ok" || phases[0] != "tx" || !closed {
		t.Fatalf("%v %v %v", ops, phases, closed)
	}
}

type metricsSpy struct {
	f func(op, res, phase string)
	d func(bool)
}

func (m *metricsSpy) KMSCall(op, res, phase string, _ time.Duration) { m.f(op, res, phase) }
func (m *metricsSpy) KMSBreaker(open bool)                           { m.d(open) }
