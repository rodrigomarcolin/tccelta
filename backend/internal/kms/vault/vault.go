package vault

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"tccelta/backend/internal/kms/config"
)

// Vault faz as operações do KMS sobre o Transit: HMAC de K_auth, verify de K_rec e assinatura do JWT.
type Vault struct {
	c   *Client
	t   Transport
	cfg config.VaultConfig
	log *slog.Logger
	m   Observer
}

// NewVault faz o login e devolve o KMS pronto.
func NewVault(ctx context.Context, cfg config.VaultConfig, log *slog.Logger, m Observer) (*Vault, error) {
	c, err := NewClient(ctx, cfg, log)
	if err != nil {
		return nil, err
	}
	return &Vault{c: c, t: c, cfg: cfg, log: log, m: m}, nil
}

// OnTransport monta um Vault sobre um transporte qualquer (testes).
func OnTransport(t Transport, cfg config.VaultConfig) *Vault {
	return &Vault{t: t, cfg: cfg, log: slog.Default()}
}

// Client devolve o cliente de baixo nível (provisionamento, rotação).
func (v *Vault) Client() *Client { return v.c }

// Ready: token válido e disjuntor fechado.
func (v *Vault) Ready() bool { return v.c != nil && v.c.Ready() }

// Close para a renovação do token.
func (v *Vault) Close() {
	if v.c != nil {
		v.c.Close()
	}
}

// Observer recebe as métricas das chamadas ao Vault (implementado por internal/metricas).
type Observer interface {
	VaultCall(operation, result string, d time.Duration)
	Breaker(open bool)
}

func (v *Vault) observe(op string, t0 time.Time, err error) {
	if v.m == nil {
		return
	}
	res := "ok"
	if err != nil {
		res = "erro"
	}
	v.m.VaultCall(op, res, time.Since(t0))
	if v.c != nil {
		v.m.Breaker(v.c.BreakerOpen())
	}
}

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

// extrai o MAC de "vault:v1:<b64>".
func parseMAC(s string) ([]byte, error) {
	p := strings.SplitN(s, ":", 3)
	if len(p) != 3 || p[1] != "v1" {
		return nil, fmt.Errorf("%w: versão/formato inesperado em hmac", ErrUnavailable)
	}
	return base64.StdEncoding.DecodeString(p[2])
}

func results(r map[string]any) ([]any, error) {
	d, _ := r["data"].(map[string]any)
	br, _ := d["batch_results"].([]any)
	if br == nil {
		return nil, fmt.Errorf("%w: resposta sem batch_results", ErrUnavailable)
	}
	return br, nil
}

// HMACAuth: HMAC-SHA256 completo com K_auth, em batch.
func (v *Vault) HMACAuth(ctx context.Context, key string, inputs [][]byte) (out [][]byte, err error) {
	t0 := time.Now()
	defer func() { v.observe("hmac_auth", t0, err) }()
	items := make([]any, len(inputs))
	for i, e := range inputs {
		items[i] = map[string]any{"input": b64(e)}
	}
	r, err := v.t.Write(ctx, fmt.Sprintf("%s/hmac/%s/sha2-256", v.cfg.MountAuth, key), map[string]any{"batch_input": items})
	if err != nil {
		return nil, translate(err)
	}
	br, err := results(r)
	if err != nil {
		return nil, err
	}
	if len(br) != len(inputs) {
		return nil, fmt.Errorf("%w: batch_results com tamanho errado", ErrUnavailable)
	}
	out = make([][]byte, len(br))
	for i, x := range br {
		m, _ := x.(map[string]any)
		if e, _ := m["error"].(string); e != "" {
			return nil, fmt.Errorf("%w: erro no item %d", ErrUnavailable, i)
		}
		s, _ := m["hmac"].(string)
		if out[i], err = parseMAC(s); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// VerifyBlocks: verify com K_rec, em batch. Erro por item vira falha do KMS, não tag inválida.
func (v *Vault) VerifyBlocks(ctx context.Context, key string, inputs, tags [][]byte) (out []bool, err error) {
	t0 := time.Now()
	defer func() { v.observe("verify_rec", t0, err) }()
	if len(inputs) != len(tags) {
		return nil, errors.New("kms: entradas e tags com tamanhos diferentes")
	}
	items := make([]any, len(inputs))
	for i := range inputs {
		items[i] = map[string]any{"input": b64(inputs[i]), "hmac": "vault:v1:" + b64(tags[i])}
	}
	r, err := v.t.Write(ctx, fmt.Sprintf("%s/verify/%s/sha2-256", v.cfg.MountRec, key), map[string]any{"batch_input": items})
	if err != nil {
		return nil, translate(err)
	}
	br, err := results(r)
	if err != nil {
		return nil, err
	}
	if len(br) != len(inputs) {
		return nil, fmt.Errorf("%w: batch_results com tamanho errado", ErrUnavailable)
	}
	out = make([]bool, len(br))
	for i, x := range br {
		m, _ := x.(map[string]any)
		if e, _ := m["error"].(string); e != "" {
			return nil, fmt.Errorf("%w: erro no item %d", ErrUnavailable, i)
		}
		out[i], _ = m["valid"].(bool)
	}
	return out, nil
}

// Sign assina a signing string do JWT com a chave Ed25519.
func (v *Vault) Sign(ctx context.Context, input []byte) (sig []byte, version int, err error) {
	t0 := time.Now()
	defer func() { v.observe("sign_jwt", t0, err) }()
	r, err := v.t.Write(ctx, fmt.Sprintf("%s/sign/%s", v.cfg.MountJWT, v.cfg.JWTKey), map[string]any{"input": b64(input)})
	if err != nil {
		return nil, 0, translate(err)
	}
	d, _ := r["data"].(map[string]any)
	s, _ := d["signature"].(string)
	p := strings.SplitN(s, ":", 3)
	if len(p) != 3 || !strings.HasPrefix(p[1], "v") {
		return nil, 0, fmt.Errorf("%w: assinatura em formato inesperado", ErrUnavailable)
	}
	version, err = strconv.Atoi(p[1][1:])
	if err != nil {
		return nil, 0, fmt.Errorf("%w: versão da assinatura", ErrUnavailable)
	}
	sig, err = base64.StdEncoding.DecodeString(p[2])
	if err != nil || len(sig) != ed25519.SignatureSize {
		return nil, 0, fmt.Errorf("%w: assinatura inválida", ErrUnavailable)
	}
	return sig, version, nil
}

// PublicKeys lê as públicas da chave JWT (só versões >= min_decryption_version).
func (v *Vault) PublicKeys(ctx context.Context) (map[int]ed25519.PublicKey, error) {
	r, err := v.t.Read(ctx, fmt.Sprintf("%s/keys/%s", v.cfg.MountJWT, v.cfg.JWTKey))
	if err != nil {
		return nil, translate(err)
	}
	d, _ := r["data"].(map[string]any)
	minV := 1
	if f, ok := d["min_decryption_version"].(float64); ok && f >= 1 {
		minV = int(f)
	}
	ks, _ := d["keys"].(map[string]any)
	out := map[int]ed25519.PublicKey{}
	for k, x := range ks {
		n, err := strconv.Atoi(k)
		if err != nil || n < minV {
			continue
		}
		m, _ := x.(map[string]any)
		pk, _ := m["public_key"].(string)
		raw, err := base64.StdEncoding.DecodeString(pk)
		if err != nil || len(raw) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("%w: chave pública v%s inválida", ErrUnavailable, k)
		}
		out[n] = ed25519.PublicKey(raw)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: nenhuma chave pública", ErrUnavailable)
	}
	return out, nil
}

// translate mantém ErrUnavailable e deixa os demais erros do Vault (ex.: 403) visíveis.
func translate(err error) error {
	if errors.Is(err, ErrUnavailable) {
		return err
	}
	var ev *VaultError
	if errors.As(err, &ev) {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return fmt.Errorf("%w: %v", ErrUnavailable, err)
}
