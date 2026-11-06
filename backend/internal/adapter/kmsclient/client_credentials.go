package kmsclient

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"net/http"
	"strconv"
	"time"

	"tccelta/backend/internal/port"
)

// VerifyClient confere o segredo do cliente no KMS (POST /v1/clients/verify). O segredo vai só no corpo da
// requisição, nunca em log, métrica ou erro.
func (c *Client) VerifyClient(ctx context.Context, clientID, secret string) (valid bool, err error) {
	t0 := time.Now()
	defer func() { c.observe(ctx, "verify_client", t0, err) }()
	var resp respClientVerify
	if err := c.call(ctx, http.MethodPost, "/v1/clients/verify", reqClientVerify{ClientID: clientID, Secret: secret}, &resp); err != nil {
		return false, err
	}
	return resp.Valid, nil
}

// SignClient assina a signing string do token de cliente com a chave `cli` (POST /v1/client-jwt/sign).
func (c *Client) SignClient(ctx context.Context, input []byte) (sig []byte, version int, err error) {
	t0 := time.Now()
	defer func() { c.observe(ctx, "sign_cli", t0, err) }()
	var resp respSign
	if err := c.call(ctx, http.MethodPost, "/v1/client-jwt/sign", reqSign{Input: b64(input)}, &resp); err != nil {
		return nil, 0, err
	}
	sig, err = base64.StdEncoding.DecodeString(resp.Signature)
	if err != nil || len(sig) != ed25519.SignatureSize || resp.Version < 1 {
		return nil, 0, unavailable("assinatura inválida")
	}
	return sig, resp.Version, nil
}

// ClientPublicKeys lê as chaves públicas vigentes da chave `cli`, por versão (GET /v1/client-jwt/keys).
func (c *Client) ClientPublicKeys(ctx context.Context) (map[int]ed25519.PublicKey, error) {
	return c.keys(ctx, "/v1/client-jwt/keys")
}

var (
	_ port.ClientVerifier = (*Client)(nil)
	_ port.ClientSigner   = (*Client)(nil)
)

func (c *Client) keys(ctx context.Context, path string) (map[int]ed25519.PublicKey, error) {
	var resp respKeys
	if err := c.call(ctx, http.MethodGet, path, nil, &resp); err != nil {
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
