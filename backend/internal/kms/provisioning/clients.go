package provisioning

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"tccelta/backend/internal/kms/vault"
)

// Erros do ciclo de vida de um cliente.
var (
	ErrInvalidClientID = errors.New("client_id inválido (^[a-z0-9][a-z0-9-]{2,63}$)")
	ErrClientUnknown   = errors.New("cliente inexistente")
	ErrClientRevoked   = errors.New("cliente revogado")
)

// Clients gerencia os clientes de client credentials no KV v2 do Vault: gera o segredo, grava só o hash e o
// devolve uma única vez a quem chamou. O segredo nunca vai a log nem a mensagem de erro.
type Clients struct {
	T     vault.Transport
	Mount string
	Log   *slog.Logger
	Now   func() time.Time
}

func (c *Clients) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Clients) log(msg string, id string) {
	if c.Log != nil {
		c.Log.Info(msg, "client_id", id)
	}
}

// Create gera o segredo e grava o registro com cas=0: falha com vault.ErrClientExists se o cliente já existe
// (mesmo revogado: o client_id não é reaproveitado).
func (c *Clients) Create(ctx context.Context, id string) (secret string, err error) {
	if !vault.ValidClientID(id) {
		return "", ErrInvalidClientID
	}
	secret, rec, err := vault.NewSecret(c.now())
	if err != nil {
		return "", err
	}
	if err := vault.WriteClient(ctx, c.T, c.Mount, id, rec, 0); err != nil {
		return "", fmt.Errorf("criar cliente %s: %w", id, err)
	}
	c.log("cliente.criado", id)
	return secret, nil
}

// Rotate troca o segredo e o sal; o antigo deixa de valer na hora. Exige cliente ativo.
func (c *Clients) Rotate(ctx context.Context, id string) (secret string, err error) {
	if !vault.ValidClientID(id) {
		return "", ErrInvalidClientID
	}
	cur, version, err := vault.ReadClient(ctx, c.T, c.Mount, id)
	if err != nil {
		return "", fmt.Errorf("ler cliente %s: %w", id, err)
	}
	switch {
	case cur == nil:
		return "", ErrClientUnknown
	case cur.Status != vault.ClientActive:
		return "", ErrClientRevoked
	}
	secret, rec, err := vault.NewSecret(c.now())
	if err != nil {
		return "", err
	}
	rec.CreatedAt = cur.CreatedAt
	if err := vault.WriteClient(ctx, c.T, c.Mount, id, rec, version); err != nil {
		return "", fmt.Errorf("rotacionar cliente %s: %w", id, err)
	}
	c.log("cliente.segredo_rotacionado", id)
	return secret, nil
}

// Revoke marca o cliente como revogado e apaga hash e sal. Idempotente; o histórico fica nas versões do KV.
func (c *Clients) Revoke(ctx context.Context, id string) error {
	if !vault.ValidClientID(id) {
		return ErrInvalidClientID
	}
	cur, version, err := vault.ReadClient(ctx, c.T, c.Mount, id)
	if err != nil {
		return fmt.Errorf("ler cliente %s: %w", id, err)
	}
	switch {
	case cur == nil:
		return ErrClientUnknown
	case cur.Status == vault.ClientRevoked:
		return nil
	}
	rec := vault.ClientRecord{Algo: cur.Algo, Status: vault.ClientRevoked, CreatedAt: cur.CreatedAt}
	if err := vault.WriteClient(ctx, c.T, c.Mount, id, rec, version); err != nil {
		return fmt.Errorf("revogar cliente %s: %w", id, err)
	}
	c.log("cliente.revogado", id)
	return nil
}

// WriteSecretFile grava o segredo em path com modo 0400, substituindo o arquivo anterior (rotação).
func WriteSecretFile(path, secret string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o400) //nolint:gosec // G304: caminho passado pelo operador na CLI de provisionamento
	if err != nil {
		return err
	}
	if _, err := f.WriteString(secret); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
