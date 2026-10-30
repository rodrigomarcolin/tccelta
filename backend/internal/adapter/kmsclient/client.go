// Package kmsclient é o adaptador HTTP do serviço KMS: implementa port.Authenticator e
// porta.JWTSigner chamando os endpoints do contrato em kms/docs/contrato-http.md.
package kmsclient

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"tccelta/backend/internal/platform/breaker"
	"tccelta/backend/internal/port"
)

// Options configuram o cliente.
type Options struct {
	URL            string
	CACert         string // PEM da CA que assina o certificado do KMS (mTLS)
	ClientCert     string
	ClientKey      string
	Timeout        time.Duration
	MaxConcurrency int
}

// Client chama o serviço KMS por HTTP (keep-alive, HTTP/2 sobre TLS, mTLS opcional).
type Client struct {
	base    string
	http    *http.Client
	timeout time.Duration
	without chan struct{}
	brk     *breaker.Breaker
	met     port.MetricsKMS

	mu          sync.Mutex
	probeAt     time.Time
	probeResult bool
}

// New cria o cliente. met pode ser nil.
func New(o Options, met port.MetricsKMS) (*Client, error) {
	if o.URL == "" {
		return nil, errors.New("KMS_URL não definida")
	}
	tr := &http.Transport{MaxIdleConnsPerHost: 64, IdleConnTimeout: 90 * time.Second, ForceAttemptHTTP2: true}
	if o.CACert != "" || o.ClientCert != "" {
		tc := &tls.Config{MinVersion: tls.VersionTLS12}
		if o.CACert != "" {
			pem, err := os.ReadFile(o.CACert)
			if err != nil {
				return nil, fmt.Errorf("KMS_CACERT: %w", err)
			}
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(pem) {
				return nil, errors.New("KMS_CACERT: PEM inválido")
			}
			tc.RootCAs = pool
		}
		if o.ClientCert != "" {
			cert, err := tls.LoadX509KeyPair(o.ClientCert, o.ClientKey)
			if err != nil {
				return nil, fmt.Errorf("KMS_CLIENT_CERT: %w", err)
			}
			tc.Certificates = []tls.Certificate{cert}
		}
		tr.TLSClientConfig = tc
	}
	if o.Timeout <= 0 {
		o.Timeout = 2 * time.Second
	}
	n := o.MaxConcurrency
	if n <= 0 {
		n = 32
	}
	return &Client{base: strings.TrimRight(o.URL, "/"), http: &http.Client{Transport: tr}, timeout: o.Timeout,
		without: make(chan struct{}, n), brk: breaker.New(5, 10*time.Second), met: met}, nil
}

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

func unavailable(format string, a ...any) error {
	return fmt.Errorf("%w: %s", port.ErrKMSUnavailable, fmt.Sprintf(format, a...))
}

// call faz uma requisição JSON com semáforo, disjuntor e timeout. Erro de rede, timeout e 5xx/429
// viram porta.ErrKMSUnavailable e contam para o disjuntor; 4xx são bug de contrato e voltam como erro comum.
func (c *Client) call(ctx context.Context, method, path string, req, resp any) error {
	if !c.brk.Allow() {
		return unavailable("disjuntor aberto")
	}
	select {
	case c.without <- struct{}{}:
		defer func() { <-c.without }()
	case <-ctx.Done():
		return unavailable("sem vaga para chamar o KMS: %v", ctx.Err())
	}
	var body []byte
	if req != nil {
		var err error
		if body, err = json.Marshal(req); err != nil {
			return err
		}
	}
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		err = c.once(ctx, method, path, body, resp)
		if err == nil || !errors.Is(err, port.ErrKMSUnavailable) || ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded) {
			break
		}
	}
	switch {
	case err == nil:
		c.brk.Success()
	case errors.Is(err, port.ErrKMSUnavailable):
		c.brk.Failure()
	default:
		c.brk.Success() // o serviço respondeu: está de pé
	}
	return err
}

func (c *Client) once(ctx context.Context, method, path string, body []byte, resp any) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	r, err := http.NewRequestWithContext(ctx, method, c.base+path, rd)
	if err != nil {
		return err
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json")
	res, err := c.http.Do(r)
	if err != nil {
		return unavailable("%v", err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return unavailable("%v", err)
	}
	switch {
	case res.StatusCode/100 == 2:
		if err := json.Unmarshal(data, resp); err != nil {
			return unavailable("resposta inválida: %v", err)
		}
		return nil
	case res.StatusCode >= 500 || res.StatusCode == http.StatusTooManyRequests:
		return unavailable("status %d", res.StatusCode)
	}
	var p problem
	_ = json.Unmarshal(data, &p)
	return fmt.Errorf("kms: status %d (%s): %s", res.StatusCode, p.Code, p.Detail)
}

func (c *Client) observe(ctx context.Context, op string, t0 time.Time, err error) {
	if c.met == nil {
		return
	}
	res := "ok"
	if err != nil {
		res = "erro"
	}
	c.met.KMSCall(op, res, string(port.KMSPhaseFrom(ctx)), time.Since(t0))
	c.met.KMSBreaker(c.brk.Open())
}

// HMACAuth: HMAC-SHA256 completo (32 B) de cada entrada com a chave K_auth de nome `chave`.
func (c *Client) HMACAuth(ctx context.Context, key string, inputs [][]byte) (out [][]byte, err error) {
	t0 := time.Now()
	defer func() { c.observe(ctx, "hmac_auth", t0, err) }()
	req := reqHMAC{Key: key, Inputs: make([]string, len(inputs))}
	for i, e := range inputs {
		req.Inputs[i] = b64(e)
	}
	var resp respHMAC
	if err := c.call(ctx, http.MethodPost, "/v1/hmac", req, &resp); err != nil {
		return nil, err
	}
	if len(resp.MACs) != len(inputs) {
		return nil, unavailable("macs com tamanho errado")
	}
	out = make([][]byte, len(resp.MACs))
	for i, m := range resp.MACs {
		if out[i], err = base64.StdEncoding.DecodeString(m); err != nil || len(out[i]) != 32 {
			return nil, unavailable("mac %d inválido", i)
		}
	}
	return out, nil
}

// VerifyBlocks: verifica tags de 32 B com a chave K_rec de nome `chave`, numa única chamada.
func (c *Client) VerifyBlocks(ctx context.Context, key string, inputs, tags [][]byte) (out []bool, err error) {
	t0 := time.Now()
	defer func() { c.observe(ctx, "verify_rec", t0, err) }()
	if len(inputs) != len(tags) {
		return nil, errors.New("kms: entradas e tags com tamanhos diferentes")
	}
	req := reqVerify{Key: key, Inputs: make([]string, len(inputs)), Tags: make([]string, len(tags))}
	for i := range inputs {
		req.Inputs[i], req.Tags[i] = b64(inputs[i]), b64(tags[i])
	}
	var resp respVerify
	if err := c.call(ctx, http.MethodPost, "/v1/blocks/verify", req, &resp); err != nil {
		return nil, err
	}
	if len(resp.Valid) != len(inputs) {
		return nil, unavailable("validos com tamanho errado")
	}
	return resp.Valid, nil
}

// Sign assina a signing string do JWT com a chave Ed25519 do KMS.
func (c *Client) Sign(ctx context.Context, input []byte) (sig []byte, version int, err error) {
	t0 := time.Now()
	defer func() { c.observe(ctx, "sign_jwt", t0, err) }()
	var resp respSign
	if err := c.call(ctx, http.MethodPost, "/v1/jwt/sign", reqSign{Input: b64(input)}, &resp); err != nil {
		return nil, 0, err
	}
	sig, err = base64.StdEncoding.DecodeString(resp.Signature)
	if err != nil || len(sig) != ed25519.SignatureSize || resp.Version < 1 {
		return nil, 0, unavailable("assinatura inválida")
	}
	return sig, resp.Version, nil
}

// PublicKeys lê as chaves públicas vigentes do JWT, por versão.
func (c *Client) PublicKeys(ctx context.Context) (map[int]ed25519.PublicKey, error) {
	var resp respKeys
	if err := c.call(ctx, http.MethodGet, "/v1/jwt/keys", nil, &resp); err != nil {
		return nil, err
	}
	out := make(map[int]ed25519.PublicKey, len(resp))
	for k, v := range resp {
		n, err := strconv.Atoi(k)
		if err != nil {
			return nil, unavailable("versão %q inválida", k)
		}
		raw, err := base64.StdEncoding.DecodeString(v)
		if err != nil || len(raw) != ed25519.PublicKeySize {
			return nil, unavailable("chave pública v%s inválida", k)
		}
		out[n] = ed25519.PublicKey(raw)
	}
	if len(out) == 0 {
		return nil, unavailable("nenhuma chave pública")
	}
	return out, nil
}

// Ready: disjuntor fechado e o /saude/pronta do KMS respondendo 200 (consulta em cache por 2 s).
func (c *Client) Ready(ctx context.Context) bool {
	if c.brk.Open() {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.probeAt) < 2*time.Second {
		return c.probeResult
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	ok := false
	if r, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/saude/pronta", nil); err == nil {
		if res, err := c.http.Do(r); err == nil {
			_, _ = io.Copy(io.Discard, res.Body)
			res.Body.Close()
			ok = res.StatusCode == http.StatusOK
		}
	}
	c.probeAt, c.probeResult = time.Now(), ok
	return ok
}

var (
	_ port.Authenticator = (*Client)(nil)
	_ port.JWTSigner     = (*Client)(nil)
)
