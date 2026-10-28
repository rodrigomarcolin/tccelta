// Package vaultteste é um Vault Transit falso, em memória, que implementa vault.Transport.
// Serve aos testes do servidor HTTP e do provisionamento.
package vaulttest

import (
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"tccelta/backend/internal/kms/vault"
)

// Fake emula os caminhos do Transit usados pelo KMS.
type Fake struct {
	mu        sync.Mutex
	auth, rec map[string][]byte
	jwt       map[int]ed25519.PrivateKey
	createdAt map[int]time.Time
	ver       int
	minDec    int
	// Failure, se não nula, é devolvida por todas as chamadas.
	Failure error
	// Clock, se não nulo, dá o creation_time das versões da chave do JWT.
	Clock func() time.Time
	// Calls conta as chamadas por caminho.
	Calls map[string]int
}

// New cria o Vault falso com a versão 1 da chave do JWT.
func New() *Fake {
	f := &Fake{auth: map[string][]byte{}, rec: map[string][]byte{}, jwt: map[int]ed25519.PrivateKey{},
		createdAt: map[int]time.Time{}, minDec: 1, Calls: map[string]int{}}
	f.rotacionar()
	return f
}

// AddDongle cadastra as chaves K_auth e K_rec com o nome dado.
func (f *Fake) AddDongle(name string, kAuth, kRec []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.auth[name], f.rec[name] = append([]byte(nil), kAuth...), append([]byte(nil), kRec...)
}

// PublicJWT devolve a chave pública da versão v.
func (f *Fake) PublicJWT(v int) ed25519.PublicKey {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.jwt[v].Public().(ed25519.PublicKey)
}

func (f *Fake) rotacionar() {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	f.ver++
	f.jwt[f.ver] = priv
	f.createdAt[f.ver] = f.now()
}

func mac(k, msg []byte) []byte {
	h := hmac.New(sha256.New, k)
	h.Write(msg)
	return h.Sum(nil)
}

func failure(status int, msg string) error {
	return &vault.VaultError{Status: status, Messages: []string{msg}}
}

func batch(body map[string]any) ([]map[string]any, error) {
	bi, _ := body["batch_input"].([]any)
	out := make([]map[string]any, len(bi))
	for i, x := range bi {
		m, ok := x.(map[string]any)
		if !ok {
			return nil, failure(400, "batch_input inválido")
		}
		out[i] = m
	}
	return out, nil
}

// Read implementa vault.Transport.
func (f *Fake) Read(_ context.Context, path string) (map[string]any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls[path]++
	if f.Failure != nil {
		return nil, f.Failure
	}
	switch {
	case path == "transit-jwt/keys/jwt":
		ks := map[string]any{}
		for v, p := range f.jwt {
			ks[strconv.Itoa(v)] = map[string]any{"public_key": base64.StdEncoding.EncodeToString(p.Public().(ed25519.PublicKey)),
				"creation_time": f.createdAt[v].Format(time.RFC3339Nano)}
		}
		return map[string]any{"data": map[string]any{"min_decryption_version": float64(f.minDec), "keys": ks}}, nil
	case strings.HasPrefix(path, "transit-auth/keys/"):
		if _, ok := f.auth[strings.TrimPrefix(path, "transit-auth/keys/")]; ok {
			return map[string]any{"data": map[string]any{"type": "hmac"}}, nil
		}
	case strings.HasPrefix(path, "transit-rec/keys/"):
		if _, ok := f.rec[strings.TrimPrefix(path, "transit-rec/keys/")]; ok {
			return map[string]any{"data": map[string]any{"type": "hmac"}}, nil
		}
	}
	return nil, failure(404, "caminho desconhecido: "+path)
}

// Write implementa vault.Transport.
func (f *Fake) Write(_ context.Context, path string, body map[string]any) (map[string]any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls[path]++
	if f.Failure != nil {
		return nil, f.Failure
	}
	p := strings.Split(path, "/")
	switch {
	case len(p) == 4 && p[0] == "transit-auth" && p[1] == "hmac":
		k, ok := f.auth[p[2]]
		if !ok {
			return nil, failure(400, "chave não encontrada")
		}
		items, err := batch(body)
		if err != nil {
			return nil, err
		}
		res := make([]any, len(items))
		for i, it := range items {
			in, err := base64.StdEncoding.DecodeString(fmt.Sprint(it["input"]))
			if err != nil {
				return nil, failure(400, "input inválido")
			}
			res[i] = map[string]any{"hmac": "vault:v1:" + base64.StdEncoding.EncodeToString(mac(k, in))}
		}
		return map[string]any{"data": map[string]any{"batch_results": res}}, nil
	case len(p) == 4 && p[0] == "transit-rec" && p[1] == "verify":
		k, ok := f.rec[p[2]]
		if !ok {
			return nil, failure(400, "chave não encontrada")
		}
		items, err := batch(body)
		if err != nil {
			return nil, err
		}
		res := make([]any, len(items))
		for i, it := range items {
			in, e1 := base64.StdEncoding.DecodeString(fmt.Sprint(it["input"]))
			tag, e2 := base64.StdEncoding.DecodeString(strings.TrimPrefix(fmt.Sprint(it["hmac"]), "vault:v1:"))
			if e1 != nil || e2 != nil {
				return nil, failure(400, "item inválido")
			}
			res[i] = map[string]any{"valid": hmac.Equal(mac(k, in), tag)}
		}
		return map[string]any{"data": map[string]any{"batch_results": res}}, nil
	case path == "transit-jwt/sign/jwt":
		in, err := base64.StdEncoding.DecodeString(fmt.Sprint(body["input"]))
		if err != nil {
			return nil, failure(400, "input inválido")
		}
		sig := ed25519.Sign(f.jwt[f.ver], in)
		return map[string]any{"data": map[string]any{"signature": fmt.Sprintf("vault:v%d:%s", f.ver, base64.StdEncoding.EncodeToString(sig))}}, nil
	case path == "transit-jwt/keys/jwt/rotate":
		f.rotacionar()
		return map[string]any{}, nil
	case path == "transit-jwt/keys/jwt/config":
		if v, ok := body["min_decryption_version"]; ok {
			f.minDec = int(v.(int))
			for k := range f.jwt {
				if k < f.minDec {
					delete(f.jwt, k)
				}
			}
		}
		return map[string]any{}, nil
	}
	return nil, failure(404, "caminho desconhecido: "+path)
}

var _ vault.Transport = (*Fake)(nil)

func (f *Fake) now() time.Time {
	if f.Clock != nil {
		return f.Clock()
	}
	return time.Now()
}
