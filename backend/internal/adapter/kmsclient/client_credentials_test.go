package kmsclient

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"

	"tccelta/backend/internal/port"
)

// Os exemplos abaixo são os de docs/kms-http-contract.md: se o contrato mudar, os dois lados mudam.
const (
	reqClientVerifyExample  = `{"client_id":"next-feira","secret":"0123456789abcdefghijklmnopqrstuvwxyzABCDEFG"}` //nolint:gosec // G101: segredo de exemplo do contrato, não é credencial
	respClientVerifyExample = `{"valid":true}`
)

func TestVerifyClient(t *testing.T) {
	var valid = true
	c, _ := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/v1/clients/verify" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("%s %s %s", r.Method, r.URL.Path, r.Header.Get("Content-Type"))
		}
		if got := bodyOf(t, r); got != reqClientVerifyExample {
			t.Errorf("corpo difere do contrato: %s", got)
		}
		if valid {
			_, _ = io.WriteString(w, respClientVerifyExample)
		} else {
			_, _ = io.WriteString(w, `{"valid":false}`)
		}
	})
	const secret = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFG"
	ok, err := c.VerifyClient(context.Background(), "next-feira", secret)
	if err != nil || !ok {
		t.Fatalf("%v %v", ok, err)
	}
	valid = false
	if ok, err := c.VerifyClient(context.Background(), "next-feira", secret); err != nil || ok {
		t.Fatalf("%v %v", ok, err)
	}
}

func TestVerifyClientFailuresAndMetrics(t *testing.T) {
	c, _ := newTest(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) })
	if _, err := c.VerifyClient(context.Background(), "next-feira", "x"); !errors.Is(err, port.ErrKMSUnavailable) {
		t.Fatal(err)
	}
	// 4xx é erro de contrato, não indisponibilidade
	c, _ = newTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = io.WriteString(w, `{"codigo":"entradas_invalidas","detail":"ruim"}`)
	})
	if _, err := c.VerifyClient(context.Background(), "next-feira", "x"); err == nil || errors.Is(err, port.ErrKMSUnavailable) {
		t.Fatalf("%v", err)
	}
	// o segredo não aparece no erro
	if _, err := c.VerifyClient(context.Background(), "next-feira", "segredo-que-nao-pode-vazar"); err != nil && bytes.Contains([]byte(err.Error()), []byte("segredo-que-nao-pode-vazar")) {
		t.Fatalf("o erro vaza o segredo: %v", err)
	}
}

func TestClientSignAndKeys(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	c, _ := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/client-jwt/sign":
			if r.Method != "POST" || bodyOf(t, r) != reqSignExample {
				t.Errorf("corpo difere do contrato")
			}
			sig := ed25519.Sign(priv, []byte("abc"))
			_ = json.NewEncoder(w).Encode(map[string]any{"signature": base64.StdEncoding.EncodeToString(sig), "version": 2})
		case "/v1/client-jwt/keys":
			if r.Method != "GET" {
				t.Errorf("método %s", r.Method)
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"2": base64.StdEncoding.EncodeToString(pub)})
		default:
			t.Errorf("caminho %s", r.URL.Path)
		}
	})
	sig, version, err := c.SignClient(context.Background(), []byte("abc"))
	if err != nil || version != 2 || !ed25519.Verify(pub, []byte("abc"), sig) {
		t.Fatalf("%v %d", err, version)
	}
	ks, err := c.ClientPublicKeys(context.Background())
	if err != nil || len(ks) != 1 || !bytes.Equal(ks[2], pub) {
		t.Fatalf("%v %v", err, ks)
	}
}

func TestClientSignAndKeysInvalidResponses(t *testing.T) {
	for name, body := range map[string]string{
		"assinatura curta": `{"signature":"AAAA","version":1}`,
		"versão zero":      `{"signature":"","version":0}`,
	} {
		c, _ := newTest(t, func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, body) })
		if _, _, err := c.SignClient(context.Background(), []byte("x")); !errors.Is(err, port.ErrKMSUnavailable) {
			t.Errorf("%s: %v", name, err)
		}
	}
	for name, body := range map[string]string{
		"vazio":           `{}`,
		"versão inválida": `{"x":"AAAA"}`,
		"chave curta":     `{"1":"AAAA"}`,
	} {
		c, _ := newTest(t, func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, body) })
		if _, err := c.ClientPublicKeys(context.Background()); !errors.Is(err, port.ErrKMSUnavailable) {
			t.Errorf("%s: %v", name, err)
		}
	}
}
