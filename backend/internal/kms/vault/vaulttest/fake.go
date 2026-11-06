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

// ring é o conjunto de versões de uma chave Ed25519 do Transit (a do app, `jwt`, ou a dos clientes, `cli`).
type ring struct {
	keys      map[int]ed25519.PrivateKey
	createdAt map[int]time.Time
	ver       int
	minDec    int
}

// kvEntry é o registro de um segredo do KV v2: os dados da última versão e o número dela.
type kvEntry struct {
	data    map[string]any
	version int
}

// Fake emula os caminhos do Transit e do KV v2 usados pelo KMS.
type Fake struct {
	mu        sync.Mutex
	auth, rec map[string][]byte
	rings     map[string]*ring // por nome de chave em transit-jwt: "jwt" e "cli"
	kv        map[string]kvEntry
	// Failure, se não nula, é devolvida por todas as chamadas.
	Failure error
	// Clock, se não nulo, dá o creation_time das versões das chaves Ed25519.
	Clock func() time.Time
	// Calls conta as chamadas por caminho.
	Calls map[string]int
}

// New cria o Vault falso com a versão 1 das chaves Ed25519 `jwt` e `cli`.
func New() *Fake {
	f := &Fake{auth: map[string][]byte{}, rec: map[string][]byte{}, rings: map[string]*ring{}, kv: map[string]kvEntry{},
		Calls: map[string]int{}}
	for _, n := range []string{"jwt", "cli"} {
		f.rings[n] = &ring{keys: map[int]ed25519.PrivateKey{}, createdAt: map[int]time.Time{}, minDec: 1}
		f.rotacionar(n)
	}
	return f
}

// AddDongle cadastra as chaves K_auth e K_rec com o nome dado.
func (f *Fake) AddDongle(name string, kAuth, kRec []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.auth[name], f.rec[name] = append([]byte(nil), kAuth...), append([]byte(nil), kRec...)
}

// PublicJWT devolve a chave pública da versão v da chave `jwt`.
func (f *Fake) PublicJWT(v int) ed25519.PublicKey { return f.public("jwt", v) }

// PublicCLI devolve a chave pública da versão v da chave `cli`.
func (f *Fake) PublicCLI(v int) ed25519.PublicKey { return f.public("cli", v) }

func (f *Fake) public(name string, v int) ed25519.PublicKey {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.rings[name].keys[v].Public().(ed25519.PublicKey)
}

// KV devolve os dados da última versão do segredo em path (ex.: "kv-clientes/data/clients/next-dev") e o número da versão.
func (f *Fake) KV(path string) (map[string]any, int, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.kv[path]
	return e.data, e.version, ok
}

func (f *Fake) rotacionar(name string) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	r := f.rings[name]
	r.ver++
	r.keys[r.ver] = priv
	r.createdAt[r.ver] = f.now()
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

func cloneMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
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
	case path == "transit-jwt/keys/jwt" || path == "transit-jwt/keys/cli":
		r := f.rings[strings.TrimPrefix(path, "transit-jwt/keys/")]
		ks := map[string]any{}
		for v, p := range r.keys {
			ks[strconv.Itoa(v)] = map[string]any{"public_key": base64.StdEncoding.EncodeToString(p.Public().(ed25519.PublicKey)),
				"creation_time": r.createdAt[v].Format(time.RFC3339Nano)}
		}
		return map[string]any{"data": map[string]any{"min_decryption_version": float64(r.minDec), "keys": ks}}, nil
	case strings.HasPrefix(path, "transit-auth/keys/"):
		if _, ok := f.auth[strings.TrimPrefix(path, "transit-auth/keys/")]; ok {
			return map[string]any{"data": map[string]any{"type": "hmac"}}, nil
		}
	case strings.HasPrefix(path, "transit-rec/keys/"):
		if _, ok := f.rec[strings.TrimPrefix(path, "transit-rec/keys/")]; ok {
			return map[string]any{"data": map[string]any{"type": "hmac"}}, nil
		}
	case strings.HasPrefix(path, "kv-clientes/data/"):
		if e, ok := f.kv[path]; ok {
			return map[string]any{"data": map[string]any{"data": cloneMap(e.data), "metadata": map[string]any{"version": float64(e.version)}}}, nil
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
	case path == "transit-jwt/sign/jwt" || path == "transit-jwt/sign/cli":
		r := f.rings[p[2]]
		in, err := base64.StdEncoding.DecodeString(fmt.Sprint(body["input"]))
		if err != nil {
			return nil, failure(400, "input inválido")
		}
		sig := ed25519.Sign(r.keys[r.ver], in)
		return map[string]any{"data": map[string]any{"signature": fmt.Sprintf("vault:v%d:%s", r.ver, base64.StdEncoding.EncodeToString(sig))}}, nil
	case len(p) == 4 && p[0] == "transit-jwt" && p[1] == "keys" && p[3] == "rotate" && f.rings[p[2]] != nil:
		f.rotacionar(p[2])
		return map[string]any{}, nil
	case len(p) == 4 && p[0] == "transit-jwt" && p[1] == "keys" && p[3] == "config" && f.rings[p[2]] != nil:
		r := f.rings[p[2]]
		if v, ok := body["min_decryption_version"]; ok {
			r.minDec = v.(int)
			for k := range r.keys {
				if k < r.minDec {
					delete(r.keys, k)
				}
			}
		}
		return map[string]any{}, nil
	case strings.HasPrefix(path, "kv-clientes/data/"):
		data, _ := body["data"].(map[string]any)
		if data == nil {
			return nil, failure(400, "no data provided")
		}
		cur := f.kv[path] // versão 0 quando não existe
		if opts, ok := body["options"].(map[string]any); ok {
			if cas, ok := opts["cas"]; ok && fmt.Sprint(cas) != strconv.Itoa(cur.version) {
				return nil, failure(400, "check-and-set parameter did not match the current version")
			}
		}
		f.kv[path] = kvEntry{data: cloneMap(data), version: cur.version + 1}
		return map[string]any{"data": map[string]any{"version": float64(cur.version + 1)}}, nil
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
