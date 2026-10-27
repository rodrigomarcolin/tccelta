// Package vaultteste é um Vault Transit falso, em memória, que implementa vault.Transporte.
// Serve aos testes do servidor HTTP e do provisionamento.
package vaultteste

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

	"tccelta/kms/internal/vault"
)

// Falso emula os caminhos do Transit usados pelo KMS.
type Falso struct {
	mu        sync.Mutex
	auth, rec map[string][]byte
	jwt       map[int]ed25519.PrivateKey
	criadas   map[int]time.Time
	ver       int
	minDec    int
	// Falha, se não nula, é devolvida por todas as chamadas.
	Falha error
	// Relogio, se não nulo, dá o creation_time das versões da chave do JWT.
	Relogio func() time.Time
	// Chamadas conta as chamadas por caminho.
	Chamadas map[string]int
}

// Novo cria o Vault falso com a versão 1 da chave do JWT.
func Novo() *Falso {
	f := &Falso{auth: map[string][]byte{}, rec: map[string][]byte{}, jwt: map[int]ed25519.PrivateKey{},
		criadas: map[int]time.Time{}, minDec: 1, Chamadas: map[string]int{}}
	f.rotacionar()
	return f
}

// AdicionarDongle cadastra as chaves K_auth e K_rec com o nome dado.
func (f *Falso) AdicionarDongle(nome string, kAuth, kRec []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.auth[nome], f.rec[nome] = append([]byte(nil), kAuth...), append([]byte(nil), kRec...)
}

// PublicaJWT devolve a chave pública da versão v.
func (f *Falso) PublicaJWT(v int) ed25519.PublicKey {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.jwt[v].Public().(ed25519.PublicKey)
}

func (f *Falso) rotacionar() {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	f.ver++
	f.jwt[f.ver] = priv
	f.criadas[f.ver] = f.agora()
}

func mac(k, msg []byte) []byte {
	h := hmac.New(sha256.New, k)
	h.Write(msg)
	return h.Sum(nil)
}

func erro(status int, msg string) error { return &vault.ErroVault{Status: status, Msgs: []string{msg}} }

func lote(corpo map[string]any) ([]map[string]any, error) {
	bi, _ := corpo["batch_input"].([]any)
	out := make([]map[string]any, len(bi))
	for i, x := range bi {
		m, ok := x.(map[string]any)
		if !ok {
			return nil, erro(400, "batch_input inválido")
		}
		out[i] = m
	}
	return out, nil
}

// Ler implementa vault.Transporte.
func (f *Falso) Ler(_ context.Context, caminho string) (map[string]any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Chamadas[caminho]++
	if f.Falha != nil {
		return nil, f.Falha
	}
	switch {
	case caminho == "transit-jwt/keys/jwt":
		ks := map[string]any{}
		for v, p := range f.jwt {
			ks[strconv.Itoa(v)] = map[string]any{"public_key": base64.StdEncoding.EncodeToString(p.Public().(ed25519.PublicKey)),
				"creation_time": f.criadas[v].Format(time.RFC3339Nano)}
		}
		return map[string]any{"data": map[string]any{"min_decryption_version": float64(f.minDec), "keys": ks}}, nil
	case strings.HasPrefix(caminho, "transit-auth/keys/"):
		if _, ok := f.auth[strings.TrimPrefix(caminho, "transit-auth/keys/")]; ok {
			return map[string]any{"data": map[string]any{"type": "hmac"}}, nil
		}
	case strings.HasPrefix(caminho, "transit-rec/keys/"):
		if _, ok := f.rec[strings.TrimPrefix(caminho, "transit-rec/keys/")]; ok {
			return map[string]any{"data": map[string]any{"type": "hmac"}}, nil
		}
	}
	return nil, erro(404, "caminho desconhecido: "+caminho)
}

// Escrever implementa vault.Transporte.
func (f *Falso) Escrever(_ context.Context, caminho string, corpo map[string]any) (map[string]any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Chamadas[caminho]++
	if f.Falha != nil {
		return nil, f.Falha
	}
	p := strings.Split(caminho, "/")
	switch {
	case len(p) == 4 && p[0] == "transit-auth" && p[1] == "hmac":
		k, ok := f.auth[p[2]]
		if !ok {
			return nil, erro(400, "chave não encontrada")
		}
		itens, err := lote(corpo)
		if err != nil {
			return nil, err
		}
		res := make([]any, len(itens))
		for i, it := range itens {
			in, err := base64.StdEncoding.DecodeString(fmt.Sprint(it["input"]))
			if err != nil {
				return nil, erro(400, "input inválido")
			}
			res[i] = map[string]any{"hmac": "vault:v1:" + base64.StdEncoding.EncodeToString(mac(k, in))}
		}
		return map[string]any{"data": map[string]any{"batch_results": res}}, nil
	case len(p) == 4 && p[0] == "transit-rec" && p[1] == "verify":
		k, ok := f.rec[p[2]]
		if !ok {
			return nil, erro(400, "chave não encontrada")
		}
		itens, err := lote(corpo)
		if err != nil {
			return nil, err
		}
		res := make([]any, len(itens))
		for i, it := range itens {
			in, e1 := base64.StdEncoding.DecodeString(fmt.Sprint(it["input"]))
			tag, e2 := base64.StdEncoding.DecodeString(strings.TrimPrefix(fmt.Sprint(it["hmac"]), "vault:v1:"))
			if e1 != nil || e2 != nil {
				return nil, erro(400, "item inválido")
			}
			res[i] = map[string]any{"valid": hmac.Equal(mac(k, in), tag)}
		}
		return map[string]any{"data": map[string]any{"batch_results": res}}, nil
	case caminho == "transit-jwt/sign/jwt":
		in, err := base64.StdEncoding.DecodeString(fmt.Sprint(corpo["input"]))
		if err != nil {
			return nil, erro(400, "input inválido")
		}
		sig := ed25519.Sign(f.jwt[f.ver], in)
		return map[string]any{"data": map[string]any{"signature": fmt.Sprintf("vault:v%d:%s", f.ver, base64.StdEncoding.EncodeToString(sig))}}, nil
	case caminho == "transit-jwt/keys/jwt/rotate":
		f.rotacionar()
		return map[string]any{}, nil
	case caminho == "transit-jwt/keys/jwt/config":
		if v, ok := corpo["min_decryption_version"]; ok {
			f.minDec = int(v.(int))
			for k := range f.jwt {
				if k < f.minDec {
					delete(f.jwt, k)
				}
			}
		}
		return map[string]any{}, nil
	}
	return nil, erro(404, "caminho desconhecido: "+caminho)
}

var _ vault.Transporte = (*Falso)(nil)

func (f *Falso) agora() time.Time {
	if f.Relogio != nil {
		return f.Relogio()
	}
	return time.Now()
}
