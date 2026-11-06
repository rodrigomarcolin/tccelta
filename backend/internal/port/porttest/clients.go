package porttest

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/subtle"

	"tccelta/backend/internal/port"
)

// AddClient cadastra um cliente de client credentials com o segredo dado.
func (m *Memory) AddClient(id, secret string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clients[id] = secret
}

// RevokeClient apaga o cliente (o KMS responde valid=false para ele).
func (m *Memory) RevokeClient(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.clients, id)
}

// RotateCLI cria a próxima versão da chave `cli`.
func (m *Memory) RotateCLI() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	m.verifyCli++
	m.cli[m.verifyCli] = priv
	return m.verifyCli
}

// PurgeCLI remove as versões < min da chave `cli`.
func (m *Memory) PurgeCLI(min int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k := range m.cli {
		if k < min {
			delete(m.cli, k)
		}
	}
}

// VerifyClient implementa port.ClientVerifier.
func (m *Memory) VerifyClient(_ context.Context, id, secret string) (bool, error) {
	m.VerifyClientCalls.Add(1)
	if err := m.failure(); err != nil {
		return false, err
	}
	m.mu.Lock()
	want, ok := m.clients[id]
	m.mu.Unlock()
	return ok && subtle.ConstantTimeCompare([]byte(want), []byte(secret)) == 1, nil
}

// SignClient implementa port.ClientSigner.
func (m *Memory) SignClient(_ context.Context, input []byte) ([]byte, int, error) {
	m.SignClientCalls.Add(1)
	if err := m.failure(); err != nil {
		return nil, 0, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return ed25519.Sign(m.cli[m.verifyCli], input), m.verifyCli, nil
}

// ClientPublicKeys implementa port.ClientSigner.
func (m *Memory) ClientPublicKeys(context.Context) (map[int]ed25519.PublicKey, error) {
	if err := m.failure(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[int]ed25519.PublicKey{}
	for k, p := range m.cli {
		out[k] = p.Public().(ed25519.PublicKey)
	}
	return out, nil
}

var (
	_ port.ClientVerifier = (*Memory)(nil)
	_ port.ClientSigner   = (*Memory)(nil)
)
