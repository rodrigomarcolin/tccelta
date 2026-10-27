package token

import (
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"tccelta/backend/internal/kms/kmsteste"
	"tccelta/backend/internal/protocolo"
)

var dev, _ = protocolo.DevIDDeHex("0011223344556677")

func novo(t *testing.T) (*kmsteste.Memoria, *Emissor, *Validador, *time.Time) {
	t.Helper()
	k := kmsteste.Nova()
	agora := time.Unix(1_800_000_000, 0)
	e := &Emissor{Assinador: k, Emissor: "iss", Audiencia: "aud", Validade: time.Hour, Agora: func() time.Time { return agora }}
	v, err := NovoValidador(context.Background(), k, "iss", "aud", 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	v.DefinirRelogio(func() time.Time { return agora })
	return k, e, v, &agora
}

func TestIdaEVolta(t *testing.T) {
	_, e, v, _ := novo(t)
	em, err := e.Emitir(context.Background(), dev, 1_700_000_007)
	if err != nil {
		t.Fatal(err)
	}
	s, err := v.Validar(context.Background(), em.Token)
	if err != nil {
		t.Fatal(err)
	}
	if s.Dev != dev || s.CS != 1_700_000_007 || s.JTI != em.JTI {
		t.Fatalf("%+v", s)
	}
}

func costurar(hd, cl map[string]any, sign func(string) string) string {
	h, _ := json.Marshal(hd)
	c, _ := json.Marshal(cl)
	a := base64.RawURLEncoding.EncodeToString(h) + "." + base64.RawURLEncoding.EncodeToString(c)
	return a + "." + sign(a)
}

func TestNegativos(t *testing.T) {
	k, e, v, agora := novo(t)
	ctx := context.Background()
	ok, _ := e.Emitir(ctx, dev, 5)
	base := func() map[string]any {
		return map[string]any{"iss": "iss", "aud": "aud", "sub": dev.Hex(), "jti": "6f1c1c1e-1d7e-4a39-9d1b-0a8b1b0f0001",
			"iat": agora.Unix(), "exp": agora.Add(time.Hour).Unix(), "cs": 5}
	}
	hdr := map[string]any{"alg": "EdDSA", "typ": "JWT", "kid": "jwt-v1"}
	semAssinar := func(string) string { return "AAAA" }
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	assinaErrado := func(a string) string { return base64.RawURLEncoding.EncodeToString(ed25519.Sign(priv, []byte(a))) }

	pubs, _ := k.ChavesPublicas(ctx)
	pub := pubs[1]
	hs := func(a string) string {
		m := hmac.New(sha256.New, pub)
		m.Write([]byte(a))
		return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
	}
	cl := base()
	cl["sub"] = "00112233445566"
	clFrac := base()
	clFrac["cs"] = 1.5
	clGrande := base()
	clGrande["cs"] = 4294967296
	clSemExp := base()
	delete(clSemExp, "exp")
	clIss := base()
	clIss["iss"] = "outro"
	clAud := base()
	clAud["aud"] = "outro"

	casos := map[string]string{
		"alg none":    costurar(map[string]any{"alg": "none", "typ": "JWT", "kid": "jwt-v1"}, base(), func(string) string { return "" }),
		"hs256":       costurar(map[string]any{"alg": "HS256", "typ": "JWT", "kid": "jwt-v1"}, base(), hs),
		"kid desc":    costurar(map[string]any{"alg": "EdDSA", "typ": "JWT", "kid": "jwt-v9"}, base(), assinaErrado),
		"kid num":     costurar(map[string]any{"alg": "EdDSA", "typ": "JWT", "kid": 1}, base(), assinaErrado),
		"iss":         costurar(hdr, clIss, assinaErrado),
		"aud":         costurar(hdr, clAud, assinaErrado),
		"sub curto":   costurar(hdr, cl, assinaErrado),
		"cs frac":     costurar(hdr, clFrac, assinaErrado),
		"cs grande":   costurar(hdr, clGrande, assinaErrado),
		"sem exp":     costurar(hdr, clSemExp, assinaErrado),
		"sem assin":   costurar(hdr, base(), semAssinar),
		"lixo":        "abc",
		"bit trocado": ok.Token[:len(ok.Token)-2] + string(rune(ok.Token[len(ok.Token)-2]^1)) + ok.Token[len(ok.Token)-1:],
	}
	for nome, tk := range casos {
		if _, err := v.Validar(ctx, tk); err != ErrTokenInvalido {
			t.Errorf("%s: esperava invalido, veio %v", nome, err)
		}
	}
	_ = agora
}

func TestExpiracaoComFolga(t *testing.T) {
	_, e, v, agora := novo(t)
	em, _ := e.Emitir(context.Background(), dev, 5)
	*agora = em.ExpiraEm.Add(29 * time.Second)
	if _, err := v.Validar(context.Background(), em.Token); err != nil {
		t.Fatalf("29 s dentro da folga: %v", err)
	}
	*agora = em.ExpiraEm.Add(31 * time.Second)
	if _, err := v.Validar(context.Background(), em.Token); err != ErrTokenExpirado {
		t.Fatalf("31 s: %v", err)
	}
}

func TestRotacao(t *testing.T) {
	k, e, v, agora := novo(t)
	ctx := context.Background()
	v1, _ := e.Emitir(ctx, dev, 5)
	k.RotacionarJWT()
	v2, err := e.Emitir(ctx, dev, 6)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(v2.Token, ".") {
		t.Fatal("token")
	}
	*agora = agora.Add(time.Minute) // passa o limite de 30 s da recarga por kid
	if _, err := v.Validar(ctx, v2.Token); err != nil {
		t.Fatalf("kid novo: %v", err)
	}
	if _, err := v.Validar(ctx, v1.Token); err != nil {
		t.Fatalf("kid antigo ainda valido: %v", err)
	}
	k.ExpurgarJWT(2)
	if err := v.recarregar(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Validar(ctx, v1.Token); err != ErrTokenInvalido {
		t.Fatalf("v1 expurgada: %v", err)
	}
}
