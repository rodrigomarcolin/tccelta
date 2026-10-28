package porttest

import (
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"sync"
	"sync/atomic"

	"tccelta/backend/internal/port"
)

// Memory guarda K_auth/K_rec por nome de chave e a chave Ed25519 do JWT, com contadores e falhas.
type Memory struct {
	mu     sync.Mutex
	auth   map[string][]byte
	rec    map[string][]byte
	jwt    map[int]ed25519.PrivateKey
	verify int

	// Contadores de chamadas.
	HMACCalls   atomic.Int64
	VerifyCalls atomic.Int64
	SignCalls   atomic.Int64
	// Failure, se não nula, é devolvida por todas as chamadas (ex.: porta.ErrKMSUnavailable).
	Failure atomic.Pointer[error]
	// FailHMACAfter, se > 0, faz HMACAuth falhar a partir da N-ésima chamada.
	FailHMACAfter atomic.Int64
}

// New cria o KMS em memória com a versão 1 da chave do JWT.
func New() *Memory {
	m := &Memory{auth: map[string][]byte{}, rec: map[string][]byte{}, jwt: map[int]ed25519.PrivateKey{}}
	m.RotateJWT()
	return m
}

// AddDongle registra K_auth/K_rec do dongle (pelo nome das chaves do cadastro).
func (m *Memory) AddDongle(authKey, recKey string, kAuth, kRec []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.auth[authKey] = append([]byte(nil), kAuth...)
	m.rec[recKey] = append([]byte(nil), kRec...)
}

// RotateJWT cria a próxima versão da chave.
func (m *Memory) RotateJWT() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	m.verify++
	m.jwt[m.verify] = priv
	return m.verify
}

// PurgeJWT remove as versões < min.
func (m *Memory) PurgeJWT(min int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k := range m.jwt {
		if k < min {
			delete(m.jwt, k)
		}
	}
}

func (m *Memory) failure() error {
	if p := m.Failure.Load(); p != nil && *p != nil {
		return *p
	}
	return nil
}

func mac(k, msg []byte) []byte {
	h := hmac.New(sha256.New, k)
	h.Write(msg)
	return h.Sum(nil)
}

// HMACAuth implementa port.Authenticator.
func (m *Memory) HMACAuth(ctx context.Context, key string, inputs [][]byte) ([][]byte, error) {
	n := m.HMACCalls.Add(1)
	if err := m.failure(); err != nil {
		return nil, err
	}
	if lim := m.FailHMACAfter.Load(); lim > 0 && n >= lim {
		return nil, port.ErrKMSUnavailable
	}
	m.mu.Lock()
	k, ok := m.auth[key]
	m.mu.Unlock()
	if !ok {
		return nil, errors.New("kmsteste: chave K_auth desconhecida")
	}
	out := make([][]byte, len(inputs))
	for i, e := range inputs {
		out[i] = mac(k, e)
	}
	return out, nil
}

// VerifyBlocks implementa port.Authenticator.
func (m *Memory) VerifyBlocks(ctx context.Context, key string, inputs, tags [][]byte) ([]bool, error) {
	m.VerifyCalls.Add(1)
	if err := m.failure(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	k, ok := m.rec[key]
	m.mu.Unlock()
	if !ok {
		return nil, errors.New("kmsteste: chave K_rec desconhecida")
	}
	out := make([]bool, len(inputs))
	for i := range inputs {
		out[i] = hmac.Equal(mac(k, inputs[i]), tags[i])
	}
	return out, nil
}

// Sign implementa porta.JWTSigner.
func (m *Memory) Sign(ctx context.Context, input []byte) ([]byte, int, error) {
	m.SignCalls.Add(1)
	if err := m.failure(); err != nil {
		return nil, 0, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return ed25519.Sign(m.jwt[m.verify], input), m.verify, nil
}

// PublicKeys implementa porta.JWTSigner.
func (m *Memory) PublicKeys(ctx context.Context) (map[int]ed25519.PublicKey, error) {
	if err := m.failure(); err != nil {
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
	_ port.Authenticator = (*Memory)(nil)
	_ port.JWTSigner     = (*Memory)(nil)
)
