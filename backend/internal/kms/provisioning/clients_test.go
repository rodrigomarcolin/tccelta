package provisioning

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tccelta/backend/internal/kms/config"
	"tccelta/backend/internal/kms/vault"
	"tccelta/backend/internal/kms/vault/vaulttest"
)

var clientsCfg = config.VaultConfig{MountJWT: "transit-jwt", JWTKey: "jwt", MountClients: "kv-clientes", ClientJWTKey: "cli"}

func newClients(t *testing.T) (*Clients, *vaulttest.Fake, *vault.Vault, *bytes.Buffer) {
	t.Helper()
	f := vaulttest.New()
	var logs bytes.Buffer
	c := &Clients{T: f, Mount: "kv-clientes", Log: slog.New(slog.NewTextHandler(&logs, nil)), Now: func() time.Time { return time.Unix(1_800_000_000, 0) }}
	return c, f, vault.OnTransport(f, clientsCfg), &logs
}

func TestClientCreateRefusesDuplicate(t *testing.T) {
	c, f, v, logs := newClients(t)
	ctx := context.Background()
	secret, err := c.Create(ctx, "next-dev")
	if err != nil || len(secret) != 43 {
		t.Fatalf("%q %v", secret, err)
	}
	if ok, _ := v.VerifyClient(ctx, "next-dev", secret); !ok {
		t.Fatal("o segredo recém-criado deveria valer")
	}
	if _, err := c.Create(ctx, "next-dev"); !errors.Is(err, vault.ErrClientExists) {
		t.Fatalf("create repetido: %v", err)
	}
	// o registro original não mudou
	if ok, _ := v.VerifyClient(ctx, "next-dev", secret); !ok {
		t.Fatal("o create repetido não pode trocar o segredo")
	}
	_, ver, _ := f.KV("kv-clientes/data/clients/next-dev")
	if ver != 1 {
		t.Fatalf("versão %d", ver)
	}
	if strings.Contains(logs.String(), secret) || !strings.Contains(logs.String(), "next-dev") {
		t.Fatalf("log: %s", logs.String())
	}
	for _, id := range []string{"ab", "A-b-c", "../x", ""} {
		if _, err := c.Create(ctx, id); !errors.Is(err, ErrInvalidClientID) {
			t.Errorf("%q: %v", id, err)
		}
	}
}

func TestClientRotateInvalidatesOldSecret(t *testing.T) {
	c, _, v, logs := newClients(t)
	ctx := context.Background()
	old, _ := c.Create(ctx, "next-dev")
	novo, err := c.Rotate(ctx, "next-dev")
	if err != nil || novo == old {
		t.Fatalf("%v", err)
	}
	if ok, _ := v.VerifyClient(ctx, "next-dev", old); ok {
		t.Fatal("o segredo antigo deveria falhar")
	}
	if ok, _ := v.VerifyClient(ctx, "next-dev", novo); !ok {
		t.Fatal("o novo deveria valer")
	}
	if strings.Contains(logs.String(), old) || strings.Contains(logs.String(), novo) {
		t.Fatalf("segredo no log: %s", logs.String())
	}
	if _, err := c.Rotate(ctx, "nao-existe"); !errors.Is(err, ErrClientUnknown) {
		t.Fatalf("inexistente: %v", err)
	}
	if err := c.Revoke(ctx, "next-dev"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Rotate(ctx, "next-dev"); !errors.Is(err, ErrClientRevoked) {
		t.Fatalf("revogado: %v", err)
	}
	if _, err := c.Rotate(ctx, "x"); !errors.Is(err, ErrInvalidClientID) {
		t.Fatalf("id inválido: %v", err)
	}
}

func TestClientRevokeIsIdempotent(t *testing.T) {
	c, f, v, _ := newClients(t)
	ctx := context.Background()
	secret, _ := c.Create(ctx, "next-dev")
	if err := c.Revoke(ctx, "next-dev"); err != nil {
		t.Fatal(err)
	}
	_, ver1, _ := f.KV("kv-clientes/data/clients/next-dev")
	if err := c.Revoke(ctx, "next-dev"); err != nil {
		t.Fatalf("segunda revogação: %v", err)
	}
	_, ver2, _ := f.KV("kv-clientes/data/clients/next-dev")
	if ver1 != ver2 {
		t.Fatalf("revogar de novo não deveria gravar outra versão: %d %d", ver1, ver2)
	}
	if ok, _ := v.VerifyClient(ctx, "next-dev", secret); ok {
		t.Fatal("nenhum token novo depois da revogação")
	}
	// o client_id revogado não é reaproveitado
	if _, err := c.Create(ctx, "next-dev"); !errors.Is(err, vault.ErrClientExists) {
		t.Fatalf("%v", err)
	}
	if err := c.Revoke(ctx, "nao-existe"); !errors.Is(err, ErrClientUnknown) {
		t.Fatalf("%v", err)
	}
	if err := c.Revoke(ctx, "!"); !errors.Is(err, ErrInvalidClientID) {
		t.Fatalf("%v", err)
	}
}

func TestClientVaultFailure(t *testing.T) {
	c, f, _, _ := newClients(t)
	f.Failure = errors.New("rede")
	if _, err := c.Create(context.Background(), "next-dev"); err == nil {
		t.Fatal("create")
	}
	if _, err := c.Rotate(context.Background(), "next-dev"); err == nil {
		t.Fatal("rotate")
	}
	if err := c.Revoke(context.Background(), "next-dev"); err == nil {
		t.Fatal("revoke")
	}
}

func TestClientNilLogAndDefaultClock(t *testing.T) {
	f := vaulttest.New()
	c := &Clients{T: f, Mount: "kv-clientes"}
	if _, err := c.Create(context.Background(), "next-dev"); err != nil {
		t.Fatal(err)
	}
}

func TestWriteSecretFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "segredo")
	if err := WriteSecretFile(p, "um"); err != nil {
		t.Fatal(err)
	}
	// rotação: substitui o arquivo (0400) sem erro
	if err := WriteSecretFile(p, "dois"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if string(b) != "dois" {
		t.Fatalf("%q", b)
	}
	if err := WriteSecretFile(filepath.Join(t.TempDir(), "nao", "existe", "x"), "x"); err == nil {
		t.Fatal("diretório inexistente")
	}
}
