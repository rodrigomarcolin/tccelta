package kmsremoto

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

	"tccelta/backend/internal/casodeuso/porta"
	"tccelta/backend/internal/casodeuso/porta/portateste"
	"tccelta/backend/internal/observabilidade"
)

// Os exemplos abaixo são os de kms/docs/contrato-http.md: se o contrato mudar, os dois lados mudam.
const (
	reqHMACExemplo = `{"chave":"0011223344556677","entradas":["YQ==","Yg=="]}`
	// os dois MACs são 32 bytes: 0x01 repetido e 0x02 repetido
	respHMACExemplo = `{"macs":["AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE=","AgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgI="]}`

	reqVerificarExemplo  = `{"chave":"0011223344556677","entradas":["YQ==","Yg=="],"tags":["dA==","dQ=="]}`
	respVerificarExemplo = `{"validos":[true,false]}`

	reqAssinarExemplo = `{"entrada":"YWJj"}`
)

func nova(t *testing.T, h http.HandlerFunc, ajustar ...func(*Opcoes)) (*Cliente, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	o := Opcoes{URL: srv.URL, Timeout: 300 * time.Millisecond, MaxConcorrencia: 4}
	for _, f := range ajustar {
		f(&o)
	}
	c, err := Novo(o, portateste.Nop{})
	if err != nil {
		t.Fatal(err)
	}
	return c, srv
}

func corpoDe(t *testing.T, r *http.Request) string {
	t.Helper()
	b, _ := io.ReadAll(r.Body)
	return string(b)
}

func TestHMACAuth(t *testing.T) {
	c, _ := nova(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/v1/hmac" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("%s %s %s", r.Method, r.URL.Path, r.Header.Get("Content-Type"))
		}
		if got := corpoDe(t, r); got != reqHMACExemplo {
			t.Errorf("corpo difere do contrato: %s", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, respHMACExemplo)
	})
	macs, err := c.HMACAuth(context.Background(), "0011223344556677", [][]byte{[]byte("a"), []byte("b")})
	if err != nil {
		t.Fatal(err)
	}
	if len(macs) != 2 || !bytes.Equal(macs[0], bytes.Repeat([]byte{1}, 32)) || !bytes.Equal(macs[1], bytes.Repeat([]byte{2}, 32)) {
		t.Fatalf("%x", macs)
	}
}

func TestVerificarBlocos(t *testing.T) {
	c, _ := nova(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/blocos/verificar" {
			t.Errorf("caminho %s", r.URL.Path)
		}
		if got := corpoDe(t, r); got != reqVerificarExemplo {
			t.Errorf("corpo difere do contrato: %s", got)
		}
		_, _ = io.WriteString(w, respVerificarExemplo)
	})
	ok, err := c.VerificarBlocos(context.Background(), "0011223344556677", [][]byte{[]byte("a"), []byte("b")}, [][]byte{[]byte("t"), []byte("u")})
	if err != nil || len(ok) != 2 || !ok[0] || ok[1] {
		t.Fatalf("%v %v", err, ok)
	}
	if _, err := c.VerificarBlocos(context.Background(), "x", [][]byte{[]byte("a")}, nil); err == nil {
		t.Fatal("tamanhos diferentes")
	}
}

func TestAssinarEChaves(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	c, _ := nova(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/jwt/assinar":
			if got := corpoDe(t, r); got != reqAssinarExemplo {
				t.Errorf("corpo difere do contrato: %s", got)
			}
			sig := ed25519.Sign(priv, []byte("abc"))
			_ = json.NewEncoder(w).Encode(map[string]any{"assinatura": base64.StdEncoding.EncodeToString(sig), "versao": 3})
		case "/v1/jwt/chaves":
			if r.Method != "GET" {
				t.Errorf("método %s", r.Method)
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"2": base64.StdEncoding.EncodeToString(pub), "3": base64.StdEncoding.EncodeToString(pub)})
		default:
			t.Errorf("caminho %s", r.URL.Path)
		}
	})
	sig, ver, err := c.Assinar(context.Background(), []byte("abc"))
	if err != nil || ver != 3 || !ed25519.Verify(pub, []byte("abc"), sig) {
		t.Fatalf("%v %d", err, ver)
	}
	ks, err := c.ChavesPublicas(context.Background())
	if err != nil || len(ks) != 2 || !bytes.Equal(ks[3], pub) {
		t.Fatalf("%v %v", err, ks)
	}
}

func TestRespostaInvalidaVirIndisponivel(t *testing.T) {
	c, _ := nova(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"macs":["AQ=="]}`) // 1 byte, e 1 item para 2 entradas
	})
	if _, err := c.HMACAuth(context.Background(), "k", [][]byte{[]byte("a"), []byte("b")}); !errors.Is(err, porta.ErrKMSIndisponivel) {
		t.Fatal(err)
	}
}

func TestFalhasViramIndisponivel(t *testing.T) {
	problema503 := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.Header().Set("Retry-After", "2")
		w.WriteHeader(503)
		_, _ = io.WriteString(w, `{"codigo":"kms_indisponivel","detail":"Vault indisponível"}`)
	}
	casos := map[string]http.HandlerFunc{
		"503":     problema503,
		"429":     func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(429) },
		"500":     func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) },
		"timeout": func(w http.ResponseWriter, r *http.Request) { time.Sleep(time.Second) },
		"json":    func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "isto não é JSON") },
	}
	for nome, h := range casos {
		c, _ := nova(t, h)
		_, err := c.HMACAuth(context.Background(), "k", [][]byte{[]byte("a")})
		if !errors.Is(err, porta.ErrKMSIndisponivel) {
			t.Errorf("%s: %v", nome, err)
		}
	}
}

func TestConexaoRecusada(t *testing.T) {
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	url := "http://" + l.Addr().String()
	_ = l.Close() // ninguém escutando
	c, err := Novo(Opcoes{URL: url, Timeout: 200 * time.Millisecond}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.HMACAuth(context.Background(), "k", [][]byte{[]byte("a")}); !errors.Is(err, porta.ErrKMSIndisponivel) {
		t.Fatal(err)
	}
	if c.Pronto() {
		t.Fatal("sem servidor não pode estar pronto")
	}
}

func TestErroDeContratoNaoEhIndisponibilidade(t *testing.T) {
	var n atomic.Int32
	c, _ := nova(t, func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		w.WriteHeader(400)
		_, _ = io.WriteString(w, `{"codigo":"chave_invalida","detail":"nome de chave inválido"}`)
	})
	_, err := c.HMACAuth(context.Background(), "k", [][]byte{[]byte("a")})
	if err == nil || errors.Is(err, porta.ErrKMSIndisponivel) {
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

func TestDisjuntorAbre(t *testing.T) {
	var n atomic.Int32
	c, _ := nova(t, func(w http.ResponseWriter, r *http.Request) { n.Add(1); w.WriteHeader(503) })
	for i := 0; i < 5; i++ {
		if _, err := c.HMACAuth(context.Background(), "k", [][]byte{[]byte("a")}); !errors.Is(err, porta.ErrKMSIndisponivel) {
			t.Fatal(err)
		}
	}
	antes := n.Load()
	_, err := c.HMACAuth(context.Background(), "k", [][]byte{[]byte("a")})
	if !errors.Is(err, porta.ErrKMSIndisponivel) || n.Load() != antes {
		t.Fatalf("com o disjuntor aberto a chamada nem chega ao servidor: %v (%d → %d)", err, antes, n.Load())
	}
	if c.Pronto() {
		t.Fatal("disjuntor aberto => não pronto")
	}
}

func TestPronto(t *testing.T) {
	var ok atomic.Bool
	ok.Store(true)
	c, _ := nova(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/saude/pronta" {
			t.Errorf("caminho %s", r.URL.Path)
		}
		if ok.Load() {
			w.WriteHeader(200)
			return
		}
		w.WriteHeader(503)
	})
	if !c.Pronto() {
		t.Fatal("deveria estar pronto")
	}
	ok.Store(false)
	c.mu.Lock()
	c.sondaEm = time.Time{} // expira o cache de 2 s
	c.mu.Unlock()
	if c.Pronto() {
		t.Fatal("KMS 503 => não pronto")
	}
}

func TestMetricasEFase(t *testing.T) {
	var (
		ops    []string
		fases  []string
		fechou bool
	)
	m := &metricasEspia{f: func(op, res, fase string) { ops, fases = append(ops, op+"/"+res), append(fases, fase) }, d: func(b bool) { fechou = !b }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, respHMACExemplo) }))
	defer srv.Close()
	c, _ := Novo(Opcoes{URL: srv.URL}, m)
	ctx := observabilidade.ComFaseKMS(context.Background(), observabilidade.FaseTx)
	if _, err := c.HMACAuth(ctx, "k", [][]byte{[]byte("a"), []byte("b")}); err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 || ops[0] != "hmac_auth/ok" || fases[0] != "tx" || !fechou {
		t.Fatalf("%v %v %v", ops, fases, fechou)
	}
}

type metricasEspia struct {
	f func(op, res, fase string)
	d func(bool)
}

func (m *metricasEspia) ChamadaKMS(op, res, fase string, _ time.Duration) { m.f(op, res, fase) }
func (m *metricasEspia) DisjuntorKMS(aberto bool)                         { m.d(aberto) }
