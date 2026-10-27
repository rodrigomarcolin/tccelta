package portateste

import (
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"sync"
	"sync/atomic"

	"tccelta/backend/internal/casodeuso/porta"
)

// Memoria guarda K_auth/K_rec por nome de chave e a chave Ed25519 do JWT, com contadores e falhas.
type Memoria struct {
	mu   sync.Mutex
	auth map[string][]byte
	rec  map[string][]byte
	jwt  map[int]ed25519.PrivateKey
	ver  int

	// Contadores de chamadas.
	ChamadasHMAC   atomic.Int64
	ChamadasVerify atomic.Int64
	ChamadasSign   atomic.Int64
	// Falha, se não nula, é devolvida por todas as chamadas (ex.: porta.ErrKMSIndisponivel).
	Falha atomic.Pointer[error]
	// FalharHMACApos, se > 0, faz HMACAuth falhar a partir da N-ésima chamada.
	FalharHMACApos atomic.Int64
}

// Nova cria o KMS em memória com a versão 1 da chave do JWT.
func Nova() *Memoria {
	m := &Memoria{auth: map[string][]byte{}, rec: map[string][]byte{}, jwt: map[int]ed25519.PrivateKey{}}
	m.RotacionarJWT()
	return m
}

// AdicionarDongle registra K_auth/K_rec do dongle (pelo nome das chaves do cadastro).
func (m *Memoria) AdicionarDongle(chaveAuth, chaveRec string, kAuth, kRec []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.auth[chaveAuth] = append([]byte(nil), kAuth...)
	m.rec[chaveRec] = append([]byte(nil), kRec...)
}

// RotacionarJWT cria a próxima versão da chave.
func (m *Memoria) RotacionarJWT() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	m.ver++
	m.jwt[m.ver] = priv
	return m.ver
}

// ExpurgarJWT remove as versões < min.
func (m *Memoria) ExpurgarJWT(min int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k := range m.jwt {
		if k < min {
			delete(m.jwt, k)
		}
	}
}

func (m *Memoria) falha() error {
	if p := m.Falha.Load(); p != nil && *p != nil {
		return *p
	}
	return nil
}

func mac(k, msg []byte) []byte {
	h := hmac.New(sha256.New, k)
	h.Write(msg)
	return h.Sum(nil)
}

// HMACAuth implementa porta.Autenticador.
func (m *Memoria) HMACAuth(ctx context.Context, chave string, entradas [][]byte) ([][]byte, error) {
	n := m.ChamadasHMAC.Add(1)
	if err := m.falha(); err != nil {
		return nil, err
	}
	if lim := m.FalharHMACApos.Load(); lim > 0 && n >= lim {
		return nil, porta.ErrKMSIndisponivel
	}
	m.mu.Lock()
	k, ok := m.auth[chave]
	m.mu.Unlock()
	if !ok {
		return nil, errors.New("kmsteste: chave K_auth desconhecida")
	}
	out := make([][]byte, len(entradas))
	for i, e := range entradas {
		out[i] = mac(k, e)
	}
	return out, nil
}

// VerificarBlocos implementa porta.Autenticador.
func (m *Memoria) VerificarBlocos(ctx context.Context, chave string, entradas, tags [][]byte) ([]bool, error) {
	m.ChamadasVerify.Add(1)
	if err := m.falha(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	k, ok := m.rec[chave]
	m.mu.Unlock()
	if !ok {
		return nil, errors.New("kmsteste: chave K_rec desconhecida")
	}
	out := make([]bool, len(entradas))
	for i := range entradas {
		out[i] = hmac.Equal(mac(k, entradas[i]), tags[i])
	}
	return out, nil
}

// Assinar implementa porta.AssinadorJWT.
func (m *Memoria) Assinar(ctx context.Context, entrada []byte) ([]byte, int, error) {
	m.ChamadasSign.Add(1)
	if err := m.falha(); err != nil {
		return nil, 0, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return ed25519.Sign(m.jwt[m.ver], entrada), m.ver, nil
}

// ChavesPublicas implementa porta.AssinadorJWT.
func (m *Memoria) ChavesPublicas(ctx context.Context) (map[int]ed25519.PublicKey, error) {
	if err := m.falha(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[int]ed25519.PublicKey{}
	for k, p := range m.jwt {
		out[k] = p.Public().(ed25519.PublicKey)
	}
	return out, nil
}

var (
	_ porta.Autenticador = (*Memoria)(nil)
	_ porta.AssinadorJWT = (*Memoria)(nil)
)
