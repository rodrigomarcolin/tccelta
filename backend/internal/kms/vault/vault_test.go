package vault_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"tccelta/backend/internal/kms/config"
	"tccelta/backend/internal/kms/vault"
	"tccelta/backend/internal/kms/vault/vaultteste"
)

var cfg = config.VaultConfig{MountAuth: "transit-auth", MountRec: "transit-rec", MountJWT: "transit-jwt", ChaveJWT: "jwt"}

func mac(k, m []byte) []byte {
	h := hmac.New(sha256.New, k)
	h.Write(m)
	return h.Sum(nil)
}

func TestOperacoes(t *testing.T) {
	f := vaultteste.Novo()
	kAuth, kRec := bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 32)
	f.AdicionarDongle("d1", kAuth, kRec)
	v := vault.SobreTransporte(f, cfg)
	ctx := context.Background()

	macs, err := v.HMACAuth(ctx, "d1", [][]byte{[]byte("a"), []byte("b")})
	if err != nil || len(macs) != 2 || !bytes.Equal(macs[1], mac(kAuth, []byte("b"))) {
		t.Fatalf("%v %x", err, macs)
	}
	ok, err := v.VerificarBlocos(ctx, "d1", [][]byte{[]byte("x"), []byte("x")}, [][]byte{mac(kRec, []byte("x")), mac(kRec, []byte("y"))})
	if err != nil || len(ok) != 2 || !ok[0] || ok[1] {
		t.Fatalf("%v %v", err, ok)
	}
	if _, err := v.VerificarBlocos(ctx, "d1", [][]byte{[]byte("x")}, nil); err == nil {
		t.Fatal("tamanhos diferentes")
	}
	sig, ver, err := v.Assinar(ctx, []byte("jwt"))
	if err != nil || ver != 1 || !ed25519.Verify(f.PublicaJWT(1), []byte("jwt"), sig) {
		t.Fatalf("%v %d", err, ver)
	}
	ks, err := v.ChavesPublicas(ctx)
	if err != nil || len(ks) != 1 || !bytes.Equal(ks[1], f.PublicaJWT(1)) {
		t.Fatalf("%v %v", err, ks)
	}
}

func TestErrosViramIndisponivel(t *testing.T) {
	f := vaultteste.Novo()
	v := vault.SobreTransporte(f, cfg)
	ctx := context.Background()
	if _, err := v.HMACAuth(ctx, "inexistente", [][]byte{[]byte("a")}); !errors.Is(err, vault.ErrIndisponivel) {
		t.Fatal(err)
	}
	f.Falha = errors.New("rede")
	if _, _, err := v.Assinar(ctx, []byte("x")); !errors.Is(err, vault.ErrIndisponivel) {
		t.Fatal(err)
	}
	if _, err := v.ChavesPublicas(ctx); !errors.Is(err, vault.ErrIndisponivel) {
		t.Fatal(err)
	}
}

func TestRotacaoEExpurgo(t *testing.T) {
	f := vaultteste.Novo()
	agora := time.Now()
	f.Relogio = func() time.Time { return agora }
	r := &vault.Rotacionador{T: f, Mount: "transit-jwt", Chave: "jwt", Periodo: 24 * time.Hour, Validade: time.Hour,
		Agora: func() time.Time { return agora }}
	ctx := context.Background()
	if rot, _, err := r.Verificar(ctx, false); err != nil || rot {
		t.Fatalf("recém-criada: %v %v", rot, err)
	}
	agora = agora.Add(25 * time.Hour)
	rot, min, err := r.Verificar(ctx, false)
	if err != nil || !rot || min != 1 {
		t.Fatalf("deveria rotacionar sem expurgar ainda: %v %v %d", err, rot, min)
	}
	v := vault.SobreTransporte(f, cfg)
	if ks, _ := v.ChavesPublicas(context.Background()); len(ks) != 2 {
		t.Fatalf("duas versões válidas: %v", ks)
	}
	// depois de Validade desde a criação da v2, a v1 é expurgada
	agora = agora.Add(2 * time.Hour)
	_, min, err = r.Verificar(ctx, false)
	if err != nil || min != 2 {
		t.Fatalf("v1 deveria sair: %v %d", err, min)
	}
	if ks, _ := v.ChavesPublicas(context.Background()); len(ks) != 1 || ks[2] == nil {
		t.Fatalf("%v", ks)
	}
}
