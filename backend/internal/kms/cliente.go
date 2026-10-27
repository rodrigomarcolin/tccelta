package kms

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

	"tccelta/backend/internal/config"
)

// ErroVault é uma resposta de erro do Vault (status != 2xx).
type ErroVault struct {
	Status int
	Msgs   []string
}

func (e *ErroVault) Error() string {
	return fmt.Sprintf("vault: status %d: %s", e.Status, strings.Join(e.Msgs, "; "))
}

// Transporte é a fronteira mínima com o Vault (facilita dublês em testes).
type Transporte interface {
	Ler(ctx context.Context, caminho string) (map[string]any, error)
	Escrever(ctx context.Context, caminho string, corpo map[string]any) (map[string]any, error)
}

// Cliente é um cliente HTTP mínimo do Vault (token estático de arquivo ou AppRole com renovação).
type Cliente struct {
	cfg     config.VaultConfig
	http    *http.Client
	log     *slog.Logger
	token   atomic.Pointer[string]
	loginMu sync.Mutex
	disj    *disjuntor
	sem     chan struct{}
	pronto  atomic.Bool
	parar   context.CancelFunc
	ttl     atomic.Int64
}

// NovoCliente cria o cliente e faz o login inicial (AppRole) ou lê o token do arquivo.
func NovoCliente(ctx context.Context, cfg config.VaultConfig, log *slog.Logger) (*Cliente, error) {
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
	n := cfg.MaxSimultaneas
	if n <= 0 {
		n = 32
	}
	c := &Cliente{cfg: cfg, http: &http.Client{Transport: tr}, log: log, disj: novoDisjuntor(5, 10*time.Second),
		sem: make(chan struct{}, n)}
	if err := c.login(ctx); err != nil {
		return nil, err
	}
	c.pronto.Store(true)
	if cfg.TokenArquivo == "" {
		lc, cancel := context.WithCancel(context.Background())
		c.parar = cancel
		go c.manter(lc)
	}
	return c, nil
}

// Pronto: token válido e disjuntor fechado.
func (c *Cliente) Pronto() bool { return c.pronto.Load() && !c.disj.aberto() }

// DisjuntorAberto para a métrica.
func (c *Cliente) DisjuntorAberto() bool { return c.disj.aberto() }

// Fechar para a renovação do token.
func (c *Cliente) Fechar() {
	if c.parar != nil {
		c.parar()
	}
}

func lerArquivo(p string) (string, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func (c *Cliente) login(ctx context.Context) error {
	c.loginMu.Lock()
	defer c.loginMu.Unlock()
	if c.cfg.TokenArquivo != "" {
		t, err := lerArquivo(c.cfg.TokenArquivo)
		if err != nil {
			return fmt.Errorf("VAULT_TOKEN_ARQUIVO: %w", err)
		}
		c.token.Store(&t)
		return nil
	}
	role, err := lerArquivo(c.cfg.RoleIDArquivo)
	if err != nil {
		return fmt.Errorf("role_id: %w", err)
	}
	sec, err := lerArquivo(c.cfg.SecretIDArquivo)
	if err != nil {
		return fmt.Errorf("secret_id: %w", err)
	}
	r, err := c.bruto(ctx, http.MethodPost, "auth/approle/login", map[string]any{"role_id": role, "secret_id": sec}, false)
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

// manter renova o token a 2/3 do TTL; se falhar, refaz o login.
func (c *Cliente) manter(ctx context.Context) {
	for ctx.Err() == nil {
		ttl := time.Duration(c.ttl.Load()) * time.Second
		if ttl <= 0 {
			ttl = 30 * time.Minute
		}
		espera := ttl * 2 / 3
		if espera < time.Second {
			espera = time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(espera):
		}
		if _, err := c.bruto(ctx, http.MethodPost, "auth/token/renew-self", map[string]any{}, true); err != nil {
			c.log.Warn("kms.renovacao_falhou_refazendo_login", "erro", err)
			if err := c.login(ctx); err != nil {
				c.pronto.Store(false)
				c.log.Error("kms.login_falhou", "erro", err)
				select {
				case <-ctx.Done():
					return
				case <-time.After(5 * time.Second):
				}
				continue
			}
			c.pronto.Store(true)
			continue
		}
		// o renew-self devolve auth.lease_duration; relê o TTL pelo lookup
		if r, err := c.bruto(ctx, http.MethodGet, "auth/token/lookup-self", nil, true); err == nil {
			if d, ok := r["data"].(map[string]any); ok {
				if t, ok := d["ttl"].(float64); ok {
					c.ttl.Store(int64(t))
				}
			}
		}
	}
}

// bruto faz a requisição HTTP, sem disjuntor.
func (c *Cliente) bruto(ctx context.Context, metodo, caminho string, corpo map[string]any, autenticar bool) (map[string]any, error) {
	var rd io.Reader
	if corpo != nil {
		b, err := json.Marshal(corpo)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(b)
	}
	ctx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, metodo, strings.TrimRight(c.cfg.Addr, "/")+"/v1/"+caminho, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if autenticar {
		if t := c.token.Load(); t != nil {
			req.Header.Set("X-Vault-Token", *t)
		}
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	dados, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		var e struct {
			Errors []string `json:"errors"`
		}
		_ = json.Unmarshal(dados, &e)
		return nil, &ErroVault{Status: resp.StatusCode, Msgs: e.Errors}
	}
	if len(dados) == 0 {
		return map[string]any{}, nil
	}
	var out map[string]any
	if err := json.Unmarshal(dados, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func transitorio(err error) bool {
	var ev *ErroVault
	if errors.As(err, &ev) {
		return ev.Status >= 500 || ev.Status == 429
	}
	return err != nil && !errors.Is(err, context.Canceled)
}

// chamar aplica semáforo, disjuntor e retentativas. 4xx (exceto 403 de token vencido) são devolvidos tal qual.
func (c *Cliente) chamar(ctx context.Context, metodo, caminho string, corpo map[string]any) (map[string]any, error) {
	if !c.disj.permitir() {
		return nil, ErrIndisponivel
	}
	select {
	case c.sem <- struct{}{}:
		defer func() { <-c.sem }()
	case <-ctx.Done():
		return nil, ErrIndisponivel
	}
	var (
		r   map[string]any
		err error
	)
	for t := 0; t <= c.cfg.Retentativas; t++ {
		r, err = c.bruto(ctx, metodo, caminho, corpo, true)
		var ev *ErroVault
		if errors.As(err, &ev) && ev.Status == 403 && c.cfg.TokenArquivo == "" && t == 0 {
			// token pode ter expirado: refaz o login uma vez
			if lerr := c.login(ctx); lerr == nil {
				continue
			}
		}
		if err == nil || !transitorio(err) {
			break
		}
		select {
		case <-ctx.Done():
		case <-time.After(time.Duration(100+t*150) * time.Millisecond):
		}
	}
	if err != nil && transitorio(err) {
		c.disj.falha()
		return nil, fmt.Errorf("%w: %v", ErrIndisponivel, err)
	}
	c.disj.sucesso() // 2xx e 4xx significam que o Vault está de pé
	return r, err
}

// Ler faz GET em /v1/<caminho>.
func (c *Cliente) Ler(ctx context.Context, caminho string) (map[string]any, error) {
	return c.chamar(ctx, http.MethodGet, caminho, nil)
}

// Escrever faz POST em /v1/<caminho>.
func (c *Cliente) Escrever(ctx context.Context, caminho string, corpo map[string]any) (map[string]any, error) {
	if corpo == nil {
		corpo = map[string]any{}
	}
	return c.chamar(ctx, http.MethodPost, caminho, corpo)
}
