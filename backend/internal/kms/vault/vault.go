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
	c   *Cliente
	t   Transporte
	cfg config.VaultConfig
	log *slog.Logger
	m   Observador
}

// NovoVault faz o login e devolve o KMS pronto.
func NovoVault(ctx context.Context, cfg config.VaultConfig, log *slog.Logger, m Observador) (*Vault, error) {
	c, err := NovoCliente(ctx, cfg, log)
	if err != nil {
		return nil, err
	}
	return &Vault{c: c, t: c, cfg: cfg, log: log, m: m}, nil
}

// SobreTransporte monta um Vault sobre um transporte qualquer (testes).
func SobreTransporte(t Transporte, cfg config.VaultConfig) *Vault {
	return &Vault{t: t, cfg: cfg, log: slog.Default()}
}

// Cliente devolve o cliente de baixo nível (provisionamento, rotação).
func (v *Vault) Cliente() *Cliente { return v.c }

// Pronto: token válido e disjuntor fechado.
func (v *Vault) Pronto() bool { return v.c != nil && v.c.Pronto() }

// Fechar para a renovação do token.
func (v *Vault) Fechar() {
	if v.c != nil {
		v.c.Fechar()
	}
}

// Observador recebe as métricas das chamadas ao Vault (implementado por internal/metricas).
type Observador interface {
	ChamadaVault(operacao, resultado string, d time.Duration)
	Disjuntor(aberto bool)
}

func (v *Vault) observar(op string, t0 time.Time, err error) {
	if v.m == nil {
		return
	}
	res := "ok"
	if err != nil {
		res = "erro"
	}
	v.m.ChamadaVault(op, res, time.Since(t0))
	if v.c != nil {
		v.m.Disjuntor(v.c.DisjuntorAberto())
	}
}

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

// extrai o MAC de "vault:v1:<b64>".
func macDe(s string) ([]byte, error) {
	p := strings.SplitN(s, ":", 3)
	if len(p) != 3 || p[1] != "v1" {
		return nil, fmt.Errorf("%w: versão/formato inesperado em hmac", ErrIndisponivel)
	}
	return base64.StdEncoding.DecodeString(p[2])
}

func resultados(r map[string]any) ([]any, error) {
	d, _ := r["data"].(map[string]any)
	br, _ := d["batch_results"].([]any)
	if br == nil {
		return nil, fmt.Errorf("%w: resposta sem batch_results", ErrIndisponivel)
	}
	return br, nil
}

// HMACAuth: HMAC-SHA256 completo com K_auth, em batch.
func (v *Vault) HMACAuth(ctx context.Context, chave string, entradas [][]byte) (out [][]byte, err error) {
	t0 := time.Now()
	defer func() { v.observar("hmac_auth", t0, err) }()
	itens := make([]any, len(entradas))
	for i, e := range entradas {
		itens[i] = map[string]any{"input": b64(e)}
	}
	r, err := v.t.Escrever(ctx, fmt.Sprintf("%s/hmac/%s/sha2-256", v.cfg.MountAuth, chave), map[string]any{"batch_input": itens})
	if err != nil {
		return nil, traduzir(err)
	}
	br, err := resultados(r)
	if err != nil {
		return nil, err
	}
	if len(br) != len(entradas) {
		return nil, fmt.Errorf("%w: batch_results com tamanho errado", ErrIndisponivel)
	}
	out = make([][]byte, len(br))
	for i, x := range br {
		m, _ := x.(map[string]any)
		if e, _ := m["error"].(string); e != "" {
			return nil, fmt.Errorf("%w: erro no item %d", ErrIndisponivel, i)
		}
		s, _ := m["hmac"].(string)
		if out[i], err = macDe(s); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// VerificarBlocos: verify com K_rec, em batch. Erro por item vira falha do KMS, não tag inválida.
func (v *Vault) VerificarBlocos(ctx context.Context, chave string, entradas, tags [][]byte) (out []bool, err error) {
	t0 := time.Now()
	defer func() { v.observar("verify_rec", t0, err) }()
	if len(entradas) != len(tags) {
		return nil, errors.New("kms: entradas e tags com tamanhos diferentes")
	}
	itens := make([]any, len(entradas))
	for i := range entradas {
		itens[i] = map[string]any{"input": b64(entradas[i]), "hmac": "vault:v1:" + b64(tags[i])}
	}
	r, err := v.t.Escrever(ctx, fmt.Sprintf("%s/verify/%s/sha2-256", v.cfg.MountRec, chave), map[string]any{"batch_input": itens})
	if err != nil {
		return nil, traduzir(err)
	}
	br, err := resultados(r)
	if err != nil {
		return nil, err
	}
	if len(br) != len(entradas) {
		return nil, fmt.Errorf("%w: batch_results com tamanho errado", ErrIndisponivel)
	}
	out = make([]bool, len(br))
	for i, x := range br {
		m, _ := x.(map[string]any)
		if e, _ := m["error"].(string); e != "" {
			return nil, fmt.Errorf("%w: erro no item %d", ErrIndisponivel, i)
		}
		out[i], _ = m["valid"].(bool)
	}
	return out, nil
}

// Assinar assina a signing string do JWT com a chave Ed25519.
func (v *Vault) Assinar(ctx context.Context, entrada []byte) (sig []byte, versao int, err error) {
	t0 := time.Now()
	defer func() { v.observar("sign_jwt", t0, err) }()
	r, err := v.t.Escrever(ctx, fmt.Sprintf("%s/sign/%s", v.cfg.MountJWT, v.cfg.ChaveJWT), map[string]any{"input": b64(entrada)})
	if err != nil {
		return nil, 0, traduzir(err)
	}
	d, _ := r["data"].(map[string]any)
	s, _ := d["signature"].(string)
	p := strings.SplitN(s, ":", 3)
	if len(p) != 3 || !strings.HasPrefix(p[1], "v") {
		return nil, 0, fmt.Errorf("%w: assinatura em formato inesperado", ErrIndisponivel)
	}
	versao, err = strconv.Atoi(p[1][1:])
	if err != nil {
		return nil, 0, fmt.Errorf("%w: versão da assinatura", ErrIndisponivel)
	}
	sig, err = base64.StdEncoding.DecodeString(p[2])
	if err != nil || len(sig) != ed25519.SignatureSize {
		return nil, 0, fmt.Errorf("%w: assinatura inválida", ErrIndisponivel)
	}
	return sig, versao, nil
}

// ChavesPublicas lê as públicas da chave JWT (só versões >= min_decryption_version).
func (v *Vault) ChavesPublicas(ctx context.Context) (map[int]ed25519.PublicKey, error) {
	r, err := v.t.Ler(ctx, fmt.Sprintf("%s/keys/%s", v.cfg.MountJWT, v.cfg.ChaveJWT))
	if err != nil {
		return nil, traduzir(err)
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
			return nil, fmt.Errorf("%w: chave pública v%s inválida", ErrIndisponivel, k)
		}
		out[n] = ed25519.PublicKey(raw)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: nenhuma chave pública", ErrIndisponivel)
	}
	return out, nil
}

// traduzir mantém ErrIndisponivel e deixa os demais erros do Vault (ex.: 403) visíveis.
func traduzir(err error) error {
	if errors.Is(err, ErrIndisponivel) {
		return err
	}
	var ev *ErroVault
	if errors.As(err, &ev) {
		return fmt.Errorf("%w: %v", ErrIndisponivel, err)
	}
	return fmt.Errorf("%w: %v", ErrIndisponivel, err)
}
