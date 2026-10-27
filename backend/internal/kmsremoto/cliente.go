// Package kmsremoto é o adaptador HTTP do serviço KMS: implementa porta.Autenticador e
// porta.AssinadorJWT chamando os endpoints do contrato em kms/docs/contrato-http.md.
package kmsremoto

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

	"tccelta/backend/internal/casodeuso/porta"
	"tccelta/backend/internal/observabilidade"
)

// Opcoes configuram o cliente.
type Opcoes struct {
	URL             string
	CACert          string // PEM da CA que assina o certificado do KMS (mTLS)
	ClientCert      string
	ClientKey       string
	Timeout         time.Duration
	MaxConcorrencia int
}

// Cliente chama o serviço KMS por HTTP (keep-alive, HTTP/2 sobre TLS, mTLS opcional).
type Cliente struct {
	base    string
	http    *http.Client
	timeout time.Duration
	sem     chan struct{}
	disj    *disjuntor
	met     porta.MetricasKMS

	mu          sync.Mutex
	sondaEm     time.Time
	sondaResult bool
}

// Novo cria o cliente. met pode ser nil.
func Novo(o Opcoes, met porta.MetricasKMS) (*Cliente, error) {
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
	n := o.MaxConcorrencia
	if n <= 0 {
		n = 32
	}
	return &Cliente{base: strings.TrimRight(o.URL, "/"), http: &http.Client{Transport: tr}, timeout: o.Timeout,
		sem: make(chan struct{}, n), disj: novoDisjuntor(5, 10*time.Second), met: met}, nil
}

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

func indisponivel(formato string, a ...any) error {
	return fmt.Errorf("%w: %s", porta.ErrKMSIndisponivel, fmt.Sprintf(formato, a...))
}

// chamar faz uma requisição JSON com semáforo, disjuntor e timeout. Erro de rede, timeout e 5xx/429
// viram porta.ErrKMSIndisponivel e contam para o disjuntor; 4xx são bug de contrato e voltam como erro comum.
func (c *Cliente) chamar(ctx context.Context, metodo, caminho string, req, resp any) error {
	if !c.disj.permitir() {
		return indisponivel("disjuntor aberto")
	}
	select {
	case c.sem <- struct{}{}:
		defer func() { <-c.sem }()
	case <-ctx.Done():
		return indisponivel("sem vaga para chamar o KMS: %v", ctx.Err())
	}
	var corpo []byte
	if req != nil {
		var err error
		if corpo, err = json.Marshal(req); err != nil {
			return err
		}
	}
	var err error
	for tent := 0; tent < 2; tent++ {
		err = c.uma(ctx, metodo, caminho, corpo, resp)
		if err == nil || !errors.Is(err, porta.ErrKMSIndisponivel) || ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded) {
			break
		}
	}
	switch {
	case err == nil:
		c.disj.sucesso()
	case errors.Is(err, porta.ErrKMSIndisponivel):
		c.disj.falha()
	default:
		c.disj.sucesso() // o serviço respondeu: está de pé
	}
	return err
}

func (c *Cliente) uma(ctx context.Context, metodo, caminho string, corpo []byte, resp any) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	var rd io.Reader
	if corpo != nil {
		rd = bytes.NewReader(corpo)
	}
	r, err := http.NewRequestWithContext(ctx, metodo, c.base+caminho, rd)
	if err != nil {
		return err
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json")
	res, err := c.http.Do(r)
	if err != nil {
		return indisponivel("%v", err)
	}
	defer res.Body.Close()
	dados, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return indisponivel("%v", err)
	}
	switch {
	case res.StatusCode/100 == 2:
		if err := json.Unmarshal(dados, resp); err != nil {
			return indisponivel("resposta inválida: %v", err)
		}
		return nil
	case res.StatusCode >= 500 || res.StatusCode == http.StatusTooManyRequests:
		return indisponivel("status %d", res.StatusCode)
	}
	var p problema
	_ = json.Unmarshal(dados, &p)
	return fmt.Errorf("kms: status %d (%s): %s", res.StatusCode, p.Codigo, p.Detail)
}

func (c *Cliente) observar(ctx context.Context, op string, t0 time.Time, err error) {
	if c.met == nil {
		return
	}
	res := "ok"
	if err != nil {
		res = "erro"
	}
	c.met.ChamadaKMS(op, res, string(observabilidade.FaseKMSDe(ctx)), time.Since(t0))
	c.met.DisjuntorKMS(c.disj.aberto())
}

// HMACAuth: HMAC-SHA256 completo (32 B) de cada entrada com a chave K_auth de nome `chave`.
func (c *Cliente) HMACAuth(ctx context.Context, chave string, entradas [][]byte) (out [][]byte, err error) {
	t0 := time.Now()
	defer func() { c.observar(ctx, "hmac_auth", t0, err) }()
	req := reqHMAC{Chave: chave, Entradas: make([]string, len(entradas))}
	for i, e := range entradas {
		req.Entradas[i] = b64(e)
	}
	var resp respHMAC
	if err := c.chamar(ctx, http.MethodPost, "/v1/hmac", req, &resp); err != nil {
		return nil, err
	}
	if len(resp.MACs) != len(entradas) {
		return nil, indisponivel("macs com tamanho errado")
	}
	out = make([][]byte, len(resp.MACs))
	for i, m := range resp.MACs {
		if out[i], err = base64.StdEncoding.DecodeString(m); err != nil || len(out[i]) != 32 {
			return nil, indisponivel("mac %d inválido", i)
		}
	}
	return out, nil
}

// VerificarBlocos: verifica tags de 32 B com a chave K_rec de nome `chave`, numa única chamada.
func (c *Cliente) VerificarBlocos(ctx context.Context, chave string, entradas, tags [][]byte) (out []bool, err error) {
	t0 := time.Now()
	defer func() { c.observar(ctx, "verify_rec", t0, err) }()
	if len(entradas) != len(tags) {
		return nil, errors.New("kms: entradas e tags com tamanhos diferentes")
	}
	req := reqVerificar{Chave: chave, Entradas: make([]string, len(entradas)), Tags: make([]string, len(tags))}
	for i := range entradas {
		req.Entradas[i], req.Tags[i] = b64(entradas[i]), b64(tags[i])
	}
	var resp respVerificar
	if err := c.chamar(ctx, http.MethodPost, "/v1/blocos/verificar", req, &resp); err != nil {
		return nil, err
	}
	if len(resp.Validos) != len(entradas) {
		return nil, indisponivel("validos com tamanho errado")
	}
	return resp.Validos, nil
}

// Assinar assina a signing string do JWT com a chave Ed25519 do KMS.
func (c *Cliente) Assinar(ctx context.Context, entrada []byte) (sig []byte, versao int, err error) {
	t0 := time.Now()
	defer func() { c.observar(ctx, "sign_jwt", t0, err) }()
	var resp respAssinar
	if err := c.chamar(ctx, http.MethodPost, "/v1/jwt/assinar", reqAssinar{Entrada: b64(entrada)}, &resp); err != nil {
		return nil, 0, err
	}
	sig, err = base64.StdEncoding.DecodeString(resp.Assinatura)
	if err != nil || len(sig) != ed25519.SignatureSize || resp.Versao < 1 {
		return nil, 0, indisponivel("assinatura inválida")
	}
	return sig, resp.Versao, nil
}

// ChavesPublicas lê as chaves públicas vigentes do JWT, por versão.
func (c *Cliente) ChavesPublicas(ctx context.Context) (map[int]ed25519.PublicKey, error) {
	var resp respChaves
	if err := c.chamar(ctx, http.MethodGet, "/v1/jwt/chaves", nil, &resp); err != nil {
		return nil, err
	}
	out := make(map[int]ed25519.PublicKey, len(resp))
	for k, v := range resp {
		n, err := strconv.Atoi(k)
		if err != nil {
			return nil, indisponivel("versão %q inválida", k)
		}
		raw, err := base64.StdEncoding.DecodeString(v)
		if err != nil || len(raw) != ed25519.PublicKeySize {
			return nil, indisponivel("chave pública v%s inválida", k)
		}
		out[n] = ed25519.PublicKey(raw)
	}
	if len(out) == 0 {
		return nil, indisponivel("nenhuma chave pública")
	}
	return out, nil
}

// Pronto: disjuntor fechado e o /saude/pronta do KMS respondendo 200 (consulta em cache por 2 s).
func (c *Cliente) Pronto() bool {
	if c.disj.aberto() {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.sondaEm) < 2*time.Second {
		return c.sondaResult
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	ok := false
	if r, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/saude/pronta", nil); err == nil {
		if res, err := c.http.Do(r); err == nil {
			_, _ = io.Copy(io.Discard, res.Body)
			res.Body.Close()
			ok = res.StatusCode == http.StatusOK
		}
	}
	c.sondaEm, c.sondaResult = time.Now(), ok
	return ok
}

var (
	_ porta.Autenticador = (*Cliente)(nil)
	_ porta.AssinadorJWT = (*Cliente)(nil)
)
