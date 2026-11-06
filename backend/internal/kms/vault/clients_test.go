package vault_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"strings"
	"testing"
	"time"

	"tccelta/backend/internal/kms/config"
	"tccelta/backend/internal/kms/vault"
	"tccelta/backend/internal/kms/vault/vaulttest"
)

var clientCfg = config.VaultConfig{MountAuth: "transit-auth", MountRec: "transit-rec", MountJWT: "transit-jwt", JWTKey: "jwt",
	MountClients: "kv-clientes", ClientJWTKey: "cli"}

func TestNewSecretFormat(t *testing.T) {
	s, rec, err := vault.NewSecret(time.Unix(1_800_000_000, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(s) != 43 || strings.ContainsAny(s, "+/=") {
		t.Fatalf("segredo de 32 B em base64url sem padding: %q", s)
	}
	if rec.Algo != "sha256-v1" || rec.Status != "active" || rec.Salt == "" || rec.Hash == "" || rec.CreatedAt != "2027-01-15T08:00:00Z" {
		t.Fatalf("%+v", rec)
	}
	s2, rec2, _ := vault.NewSecret(time.Now())
	if s == s2 || rec.Salt == rec2.Salt {
		t.Fatal("segredo e sal devem ser aleatórios")
	}
	if got := vault.SecretHash([]byte("sal"), "x"); len(got) != 32 {
		t.Fatalf("%x", got)
	}
}

func TestVerifyClient(t *testing.T) {
	f := vaulttest.New()
	secret, rec, _ := vault.NewSecret(time.Now())
	ctx := context.Background()
	if err := vault.WriteClient(ctx, f, "kv-clientes", "next-dev", rec, 0); err != nil {
		t.Fatal(err)
	}
	v := vault.OnTransport(f, clientCfg)
	for name, c := range map[string]struct {
		id, secret string
		want       bool
	}{
		"certo":       {"next-dev", secret, true},
		"errado":      {"next-dev", secret + "x", false},
		"vazio":       {"next-dev", "", false},
		"inexistente": {"outro-dev", secret, false},
	} {
		got, err := v.VerifyClient(ctx, c.id, c.secret)
		if err != nil || got != c.want {
			t.Errorf("%s: %v %v", name, got, err)
		}
	}
	// o Vault guarda só o hash: o segredo não está em nenhum campo do registro
	data, _, _ := f.KV("kv-clientes/data/clients/next-dev")
	for k, val := range data {
		if s, _ := val.(string); strings.Contains(s, secret) {
			t.Errorf("campo %s guarda o segredo", k)
		}
	}
	// revogado: nenhum segredo vale, e o registro perde hash e sal
	rev := vault.ClientRecord{Algo: rec.Algo, Status: vault.ClientRevoked, CreatedAt: rec.CreatedAt}
	if err := vault.WriteClient(ctx, f, "kv-clientes", "next-dev", rev, 1); err != nil {
		t.Fatal(err)
	}
	if ok, err := v.VerifyClient(ctx, "next-dev", secret); err != nil || ok {
		t.Fatalf("revogado: %v %v", ok, err)
	}
	data, _, _ = f.KV("kv-clientes/data/clients/next-dev")
	if _, has := data["hash"]; has || data["status"] != "revoked" {
		t.Fatalf("%v", data)
	}
}

func TestVerifyClientCorruptRecordIsInvalid(t *testing.T) {
	for name, rec := range map[string]vault.ClientRecord{
		"algoritmo desconhecido": {Algo: "md5", Salt: "AAAA", Hash: "AAAA", Status: "active"},
		"hash curto":             {Algo: "sha256-v1", Salt: "AAAA", Hash: "AAAA", Status: "active"},
		"base64 ruim":            {Algo: "sha256-v1", Salt: "!!", Hash: "!!", Status: "active"},
	} {
		f := vaulttest.New()
		if err := vault.WriteClient(context.Background(), f, "kv-clientes", "ruim-dev", rec, 0); err != nil {
			t.Fatal(name, err)
		}
		if ok, err := vault.OnTransport(f, clientCfg).VerifyClient(context.Background(), "ruim-dev", "qualquer"); err != nil || ok {
			t.Errorf("%s: %v %v", name, ok, err)
		}
	}
}

func TestVerifyClientVaultFailure(t *testing.T) {
	f := vaulttest.New()
	f.Failure = errors.New("rede")
	if _, err := vault.OnTransport(f, clientCfg).VerifyClient(context.Background(), "next-dev", "x"); !errors.Is(err, vault.ErrUnavailable) {
		t.Fatal(err)
	}
	// 403 do Vault (política) também é indisponibilidade, nunca "inválido"
	f.Failure = &vault.VaultError{Status: 403, Messages: []string{"permission denied"}}
	if _, err := vault.OnTransport(f, clientCfg).VerifyClient(context.Background(), "next-dev", "x"); !errors.Is(err, vault.ErrUnavailable) {
		t.Fatal(err)
	}
}

func TestWriteClientCAS(t *testing.T) {
	f := vaulttest.New()
	ctx := context.Background()
	_, rec, _ := vault.NewSecret(time.Now())
	if err := vault.WriteClient(ctx, f, "kv-clientes", "next-dev", rec, 0); err != nil {
		t.Fatal(err)
	}
	if err := vault.WriteClient(ctx, f, "kv-clientes", "next-dev", rec, 0); !errors.Is(err, vault.ErrClientExists) {
		t.Fatalf("create repetido: %v", err)
	}
	if err := vault.WriteClient(ctx, f, "kv-clientes", "next-dev", rec, 7); err == nil || errors.Is(err, vault.ErrClientExists) {
		t.Fatalf("versão errada: %v", err)
	}
	got, ver, err := vault.ReadClient(ctx, f, "kv-clientes", "next-dev")
	if err != nil || ver != 1 || got == nil || got.Hash != rec.Hash {
		t.Fatalf("%+v %d %v", got, ver, err)
	}
	if got, _, err := vault.ReadClient(ctx, f, "kv-clientes", "nao-existe"); err != nil || got != nil {
		t.Fatalf("inexistente: %+v %v", got, err)
	}
	// erro que não é de check-and-set sobe como veio
	f.Failure = &vault.VaultError{Status: 403, Messages: []string{"permission denied"}}
	if err := vault.WriteClient(ctx, f, "kv-clientes", "outro-dev", rec, 0); err == nil || errors.Is(err, vault.ErrClientExists) {
		t.Fatalf("%v", err)
	}
	if _, _, err := vault.ReadClient(ctx, f, "kv-clientes", "next-dev"); err == nil {
		t.Fatal("leitura com 403 deveria falhar")
	}
}

func TestClientKeySignAndKeys(t *testing.T) {
	f := vaulttest.New()
	v := vault.OnTransport(f, clientCfg)
	ctx := context.Background()
	sig, ver, err := v.SignClient(ctx, []byte("cli"))
	if err != nil || ver != 1 || !ed25519.Verify(f.PublicCLI(1), []byte("cli"), sig) {
		t.Fatalf("%v %d", err, ver)
	}
	ks, err := v.ClientPublicKeys(ctx)
	if err != nil || len(ks) != 1 || !bytes.Equal(ks[1], f.PublicCLI(1)) {
		t.Fatalf("%v %v", err, ks)
	}
	// a chave do app segue separada
	sigApp, _, _ := v.Sign(ctx, []byte("cli"))
	if ed25519.Verify(f.PublicCLI(1), []byte("cli"), sigApp) {
		t.Fatal("as duas chaves não podem coincidir")
	}
	f.Failure = errors.New("rede")
	if _, _, err := v.SignClient(ctx, []byte("x")); !errors.Is(err, vault.ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := v.ClientPublicKeys(ctx); !errors.Is(err, vault.ErrUnavailable) {
		t.Fatal(err)
	}
}

// A mesma tabela de casos do Rotator vale para a chave cli, e a rotação de uma não toca a outra.
func TestRotationAndPurgeCLI(t *testing.T) {
	f := vaulttest.New()
	now := time.Now()
	f.Clock = func() time.Time { return now }
	r := &vault.Rotator{T: f, Mount: "transit-jwt", Key: "cli", Period: 24 * time.Hour, Validity: time.Hour, Now: func() time.Time { return now }}
	ctx := context.Background()
	if rot, _, err := r.Check(ctx, false); err != nil || rot {
		t.Fatalf("recém-criada: %v %v", rot, err)
	}
	now = now.Add(25 * time.Hour)
	rot, min, err := r.Check(ctx, false)
	if err != nil || !rot || min != 1 {
		t.Fatalf("deveria rotacionar sem expurgar ainda: %v %v %d", err, rot, min)
	}
	v := vault.OnTransport(f, clientCfg)
	if ks, _ := v.ClientPublicKeys(ctx); len(ks) != 2 {
		t.Fatalf("duas versões válidas: %v", ks)
	}
	if ks, _ := v.PublicKeys(ctx); len(ks) != 1 {
		t.Fatalf("a chave jwt não pode ter girado: %v", ks)
	}
	now = now.Add(2 * time.Hour)
	if _, min, err = r.Check(ctx, false); err != nil || min != 2 {
		t.Fatalf("v1 deveria sair: %v %d", err, min)
	}
	if ks, _ := v.ClientPublicKeys(ctx); len(ks) != 1 || ks[2] == nil {
		t.Fatalf("%v", ks)
	}
	// --now: força a rotação mesmo sem vencer
	if rot, _, err := r.Check(ctx, true); err != nil || !rot {
		t.Fatalf("--now: %v %v", rot, err)
	}
	if ks, _ := v.ClientPublicKeys(ctx); len(ks) != 2 || ks[3] == nil {
		t.Fatalf("%v", ks)
	}
}

func TestValidClientID(t *testing.T) {
	for id, want := range map[string]bool{"next-dev": true, "abc": true, "a-1": true, "ab": false, "Abc": false, "-ab": false, "a/b": false,
		"": false, "..": false, strings.Repeat("a", 64): true, strings.Repeat("a", 65): false} {
		if vault.ValidClientID(id) != want {
			t.Errorf("%q", id)
		}
	}
}
