package vault

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"tccelta/backend/internal/kms/breaker"
	"tccelta/backend/internal/kms/config"
)

// VaultError é uma resposta de erro do Vault (status != 2xx).
type VaultError struct {
	Status   int
	Messages []string
}

func (e *VaultError) Error() string {
	return fmt.Sprintf("vault: status %d: %s", e.Status, strings.Join(e.Messages, "; "))
}

// Transport é a fronteira mínima com o Vault (facilita dublês em testes).
type Transport interface {
	Read(ctx context.Context, path string) (map[string]any, error)
	Write(ctx context.Context, path string, body map[string]any) (map[string]any, error)
}

// Client é um cliente HTTP mínimo do Vault (token estático de arquivo ou AppRole com renovação).
type Client struct {
	cfg     config.VaultConfig
	http    *http.Client
	log     *slog.Logger
	token   atomic.Pointer[string]
	loginMu sync.Mutex
	brk     *breaker.Breaker
	sem     chan struct{}
	ready   atomic.Bool
	stop    context.CancelFunc
	ttl     atomic.Int64
}

// NewClient cria o cliente e faz o login inicial (AppRole) ou lê o token do arquivo.
func NewClient(ctx context.Context, cfg config.VaultConfig, log *slog.Logger) (*Client, error) {
	tr := &http.Transport{MaxIdleConnsPerHost: 64, IdleConnTimeout: 90 * time.Second}
	if cfg.CACert != "" || cfg.ClientCert != "" {
		tc := &tls.Config{MinVersion: tls.VersionTLS12}
		if cfg.CACert != "" {
			pem, err := os.ReadFile(cfg.CACert)
			if err != nil {
				return nil, fmt.Errorf("VAULT_CACERT: %w", err)
			}
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(pem) {
				return nil, errors.New("VAULT_CACERT: PEM inválido")
			}
			tc.RootCAs = pool
		}
		if cfg.ClientCert != "" {
			cert, err := tls.LoadX509KeyPair(cfg.ClientCert, cfg.ClientKey)
			if err != nil {
				return nil, err
			}
			tc.Certificates = []tls.Certificate{cert}
		}
		tr.TLSClientConfig = tc
	}
	n := cfg.MaxConcurrent
	if n <= 0 {
		n = 32
	}
	c := &Client{cfg: cfg, http: &http.Client{Transport: tr}, log: log, brk: breaker.New(5, 10*time.Second),
		sem: make(chan struct{}, n)}
	if err := c.login(ctx); err != nil {
		return nil, err
	}
	c.ready.Store(true)
	if cfg.TokenFile == "" {
		lc, cancel := context.WithCancel(context.WithoutCancel(ctx))
		c.stop = cancel
		go c.keepAlive(lc)
	}
	return c, nil
}

// Ready: token válido e disjuntor fechado.
func (c *Client) Ready() bool { return c.ready.Load() && !c.brk.Open() }

// BreakerOpen para a métrica.
func (c *Client) BreakerOpen() bool { return c.brk.Open() }

// Close para a renovação do token.
func (c *Client) Close() {
	if c.stop != nil {
		c.stop()
	}
}

func readFile(p string) (string, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func (c *Client) login(ctx context.Context) error {
	c.loginMu.Lock()
	defer c.loginMu.Unlock()
	if c.cfg.TokenFile != "" {
		t, err := readFile(c.cfg.TokenFile)
		if err != nil {
			return fmt.Errorf("VAULT_TOKEN_ARQUIVO: %w", err)
		}
		c.token.Store(&t)
		return nil
	}
	role, err := readFile(c.cfg.RoleIDFile)
	if err != nil {
		return fmt.Errorf("role_id: %w", err)
	}
	sec, err := readFile(c.cfg.SecretIDFile)
	if err != nil {
		return fmt.Errorf("secret_id: %w", err)
	}
	r, err := c.raw(ctx, http.MethodPost, "auth/approle/login", map[string]any{"role_id": role, "secret_id": sec}, false)
	if err != nil {
		return fmt.Errorf("login AppRole: %w", err)
	}
	auth, _ := r["auth"].(map[string]any)
	tok, _ := auth["client_token"].(string)
	if tok == "" {
		return errors.New("login AppRole sem client_token")
	}
	c.token.Store(&tok)
	ttl, _ := auth["lease_duration"].(float64)
	c.ttl.Store(int64(ttl))
	return nil
}

// keepAlive renova o token a 2/3 do TTL; se falhar, refaz o login.
func (c *Client) keepAlive(ctx context.Context) {
	for ctx.Err() == nil {
		ttl := time.Duration(c.ttl.Load()) * time.Second
		if ttl <= 0 {
			ttl = 30 * time.Minute
		}
		wait := ttl * 2 / 3
		if wait < time.Second {
			wait = time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		if _, err := c.raw(ctx, http.MethodPost, "auth/token/renew-self", map[string]any{}, true); err != nil {
			c.log.Warn("kms.renovacao_falhou_refazendo_login", "erro", err)
			if err := c.login(ctx); err != nil {
				c.ready.Store(false)
				c.log.Error("kms.login_falhou", "erro", err)
				select {
				case <-ctx.Done():
					return
				case <-time.After(5 * time.Second):
				}
				continue
			}
			c.ready.Store(true)
			continue
		}
		// o renew-self devolve auth.lease_duration; relê o TTL pelo lookup
		if r, err := c.raw(ctx, http.MethodGet, "auth/token/lookup-self", nil, true); err == nil {
			if d, ok := r["data"].(map[string]any); ok {
				if t, ok := d["ttl"].(float64); ok {
					c.ttl.Store(int64(t))
				}
			}
		}
	}
}

// raw faz a requisição HTTP, sem disjuntor.
func (c *Client) raw(ctx context.Context, method, path string, body map[string]any, authenticate bool) (map[string]any, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(b)
	}
	ctx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.cfg.Addr, "/")+"/v1/"+path, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if authenticate {
		if t := c.token.Load(); t != nil {
			req.Header.Set("X-Vault-Token", *t)
		}
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		var e struct {
			Errors []string `json:"errors"`
		}
		_ = json.Unmarshal(data, &e)
		return nil, &VaultError{Status: resp.StatusCode, Messages: e.Errors}
	}
	if len(data) == 0 {
		return map[string]any{}, nil
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func transient(err error) bool {
	var ev *VaultError
	if errors.As(err, &ev) {
		return ev.Status >= 500 || ev.Status == 429
	}
	return err != nil && !errors.Is(err, context.Canceled)
}

// call aplica semáforo, disjuntor e retentativas. 4xx (exceto 403 de token vencido) são devolvidos tal qual.
func (c *Client) call(ctx context.Context, method, path string, body map[string]any) (map[string]any, error) {
	if !c.brk.Allow() {
		return nil, ErrUnavailable
	}
	select {
	case c.sem <- struct{}{}:
		defer func() { <-c.sem }()
	case <-ctx.Done():
		return nil, ErrUnavailable
	}
	var (
		r   map[string]any
		err error
	)
	for t := 0; t <= c.cfg.Retries; t++ {
		r, err = c.raw(ctx, method, path, body, true)
		var ev *VaultError
		if errors.As(err, &ev) && ev.Status == 403 && c.cfg.TokenFile == "" && t == 0 {
			// token pode ter expirado: refaz o login uma vez
			if loginErr := c.login(ctx); loginErr == nil {
				continue
			}
		}
		if err == nil || !transient(err) {
			break
		}
		select {
		case <-ctx.Done():
		case <-time.After(time.Duration(100+t*150) * time.Millisecond):
		}
	}
	if err != nil && transient(err) {
		c.brk.Failure()
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	c.brk.Success() // 2xx e 4xx significam que o Vault está de pé
	return r, err
}

// Read faz GET em /v1/<caminho>.
func (c *Client) Read(ctx context.Context, path string) (map[string]any, error) {
	return c.call(ctx, http.MethodGet, path, nil)
}

// Write faz POST em /v1/<caminho>.
func (c *Client) Write(ctx context.Context, path string, body map[string]any) (map[string]any, error) {
	if body == nil {
		body = map[string]any{}
	}
	return c.call(ctx, http.MethodPost, path, body)
}
