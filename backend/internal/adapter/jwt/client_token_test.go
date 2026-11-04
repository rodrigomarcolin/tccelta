package jwt

import (
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"tccelta/backend/internal/port"
	"tccelta/backend/internal/port/porttest"
)

func newClientTest(t *testing.T) (*porttest.Memory, *ClientIssuer, *ClientValidator, *time.Time) {
	t.Helper()
	k := porttest.New()
	now := time.Unix(1_800_000_000, 0)
	e := NewClientIssuer(k, "iss", "aud-analise", 30*time.Minute)
	e.SetClock(func() time.Time { return now })
	v, err := NewClientValidator(context.Background(), k, "iss", "aud-analise", 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	v.SetClock(func() time.Time { return now })
	return k, e, v, &now
}

func decodeSegment(t *testing.T, s string) map[string]any {
	t.Helper()
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestClientRoundTripAndClaims(t *testing.T) {
	k, e, v, now := newClientTest(t)
	ctx := context.Background()
	em, err := e.Issue(ctx, "next-feira")
	if err != nil {
		t.Fatal(err)
	}
	if e.Validity() != 30*time.Minute || !em.ExpiresAt.Equal(now.Add(30*time.Minute)) {
		t.Fatalf("%v %v", e.Validity(), em.ExpiresAt)
	}
	parts := strings.Split(em.Token, ".")
	hd, cl := decodeSegment(t, parts[0]), decodeSegment(t, parts[1])
	if hd["alg"] != "EdDSA" || hd["typ"] != "JWT" || hd["kid"] != "cli-v1" {
		t.Fatalf("%v", hd)
	}
	for _, c := range []string{"iss", "aud", "sub", "jti", "iat", "nbf", "exp"} {
		if _, ok := cl[c]; !ok {
			t.Errorf("falta a claim %s", c)
		}
	}
	if _, has := cl["cs"]; has {
		t.Error("o token de cliente não tem cs")
	}
	if cl["sub"] != "next-feira" || cl["aud"] != "aud-analise" || cl["iss"] != "iss" || cl["jti"] != em.JTI.String() {
		t.Fatalf("%v", cl)
	}
	// a assinatura verifica com a chave pública (a mesma que o JWKS publica)
	pubs, _ := k.ClientPublicKeys(ctx)
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	if !ed25519.Verify(pubs[1], []byte(parts[0]+"."+parts[1]), sig) {
		t.Fatal("assinatura não confere com a chave cli")
	}
	s, err := v.Validate(ctx, em.Token)
	if err != nil || s.ClientID != "next-feira" || s.JTI != em.JTI || !s.ExpiresAt.Equal(em.ExpiresAt) {
		t.Fatalf("%+v %v", s, err)
	}
}

func TestClientValidatorNegatives(t *testing.T) {
	k, e, v, now := newClientTest(t)
	ctx := context.Background()
	ok, _ := e.Issue(ctx, "next-feira")
	base := func() map[string]any {
		return map[string]any{"iss": "iss", "aud": "aud-analise", "sub": "next-feira", "jti": "6f1c1c1e-1d7e-4a39-9d1b-0a8b1b0f0001",
			"iat": now.Unix(), "exp": now.Add(time.Hour).Unix()}
	}
	hdr := map[string]any{"alg": "EdDSA", "typ": "JWT", "kid": "cli-v1"}
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	wrongSig := func(a string) string { return base64.RawURLEncoding.EncodeToString(ed25519.Sign(priv, []byte(a))) }
	pubs, _ := k.ClientPublicKeys(ctx)
	hs := func(a string) string {
		m := hmac.New(sha256.New, pubs[1])
		m.Write([]byte(a))
		return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
	}
	with := func(k string, val any) map[string]any {
		m := base()
		if val == nil {
			delete(m, k)
		} else {
			m[k] = val
		}
		return m
	}
	cases := map[string]string{
		"alg none":         stitch(map[string]any{"alg": "none", "typ": "JWT", "kid": "cli-v1"}, base(), func(string) string { return "" }),
		"hs256":            stitch(map[string]any{"alg": "HS256", "typ": "JWT", "kid": "cli-v1"}, base(), hs),
		"kid desconhecido": stitch(map[string]any{"alg": "EdDSA", "typ": "JWT", "kid": "cli-v9"}, base(), wrongSig),
		"kid do app":       stitch(map[string]any{"alg": "EdDSA", "typ": "JWT", "kid": "jwt-v1"}, base(), wrongSig),
		"sem kid":          stitch(map[string]any{"alg": "EdDSA", "typ": "JWT"}, base(), wrongSig),
		"kid numérico":     stitch(map[string]any{"alg": "EdDSA", "typ": "JWT", "kid": 1}, base(), wrongSig),
		"iss errada":       stitch(hdr, with("iss", "outro"), wrongSig),
		"aud errada":       stitch(hdr, with("aud", "tccelta-ingestao"), wrongSig),
		"sem exp":          stitch(hdr, with("exp", nil), wrongSig),
		"assinatura":       stitch(hdr, base(), wrongSig),
		"sem assinatura":   stitch(hdr, base(), func(string) string { return "AAAA" }),
		"lixo":             "abc",
		"bit trocado":      ok.Token[:len(ok.Token)-2] + string(rune(ok.Token[len(ok.Token)-2]^1)) + ok.Token[len(ok.Token)-1:],
	}
	for name, tk := range cases {
		if _, err := v.Validate(ctx, tk); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("%s: esperava inválido, veio %v", name, err)
		}
	}
	// claims erradas com assinatura boa: o emissor não produz, então assina-se com a chave do KMS de teste
	sign := func(m map[string]any) string {
		h, _ := json.Marshal(hdr)
		c, _ := json.Marshal(m)
		a := base64.RawURLEncoding.EncodeToString(h) + "." + base64.RawURLEncoding.EncodeToString(c)
		sig, _, _ := k.SignClient(ctx, []byte(a))
		return a + "." + base64.RawURLEncoding.EncodeToString(sig)
	}
	if _, err := v.Validate(ctx, sign(base())); err != nil {
		t.Fatalf("claims de referência devem passar: %v", err)
	}
	for name, m := range map[string]map[string]any{
		"sub vazio":         with("sub", ""),
		"sub sem formato":   with("sub", "Next Feira"),
		"sub com maiúscula": with("sub", "Dev_01"),
		"sem sub":           with("sub", nil),
		"jti não uuid":      with("jti", "abc"),
		"sem jti":           with("jti", nil),
	} {
		if _, err := v.Validate(ctx, sign(m)); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestClientExpirationWithLeeway(t *testing.T) {
	_, e, v, now := newClientTest(t)
	em, _ := e.Issue(context.Background(), "next-feira")
	*now = em.ExpiresAt.Add(29 * time.Second)
	if _, err := v.Validate(context.Background(), em.Token); err != nil {
		t.Fatalf("29 s dentro da folga: %v", err)
	}
	*now = em.ExpiresAt.Add(31 * time.Second)
	if _, err := v.Validate(context.Background(), em.Token); !errors.Is(err, ErrTokenExpired) {
		t.Fatalf("31 s: %v", err)
	}
}

func TestClientRotation(t *testing.T) {
	k, e, v, now := newClientTest(t)
	ctx := context.Background()
	v1, _ := e.Issue(ctx, "next-feira")
	k.RotateCLI()
	v2, err := e.Issue(ctx, "next-feira")
	if err != nil {
		t.Fatal(err)
	}
	if kid := decodeSegment(t, strings.Split(v2.Token, ".")[0])["kid"]; kid != "cli-v2" {
		t.Fatalf("o kid acompanha a versão do KMS: %v", kid)
	}
	*now = now.Add(time.Minute) // passa o limite de 30 s da recarga por kid
	if _, err := v.Validate(ctx, v2.Token); err != nil {
		t.Fatalf("kid novo: %v", err)
	}
	if _, err := v.Validate(ctx, v1.Token); err != nil {
		t.Fatalf("kid antigo ainda válido: %v", err)
	}
	if ks := v.ClientKeys(); len(ks) != 2 || ks[0].KID != "cli-v1" || ks[1].KID != "cli-v2" {
		t.Fatalf("chaves ordenadas pelo kid: %+v", ks)
	}
	k.PurgeCLI(2)
	if err := v.reload(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Validate(ctx, v1.Token); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("v1 expurgada: %v", err)
	}
	if ks := v.ClientKeys(); len(ks) != 1 || ks[0].KID != "cli-v2" {
		t.Fatalf("%+v", ks)
	}
}

func TestClientValidatorReloadByKidIsRateLimited(t *testing.T) {
	k, e, v, now := newClientTest(t)
	ctx := context.Background()
	k.RotateCLI()
	novo, _ := e.Issue(ctx, "next-feira")
	v.lastReload = *now // o boot carregou as chaves neste instante
	// logo após o boot (menos de 30 s), a recarga por kid não acontece
	if _, err := v.Validate(ctx, novo.Token); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("kid novo dentro dos 30 s: %v", err)
	}
	*now = now.Add(31 * time.Second)
	if _, err := v.Validate(ctx, novo.Token); err != nil {
		t.Fatalf("depois de 30 s a recarga acontece: %v", err)
	}
}

func TestClientIssuerRedoesWhenVersionChangesDuringIssue(t *testing.T) {
	k := porttest.New()
	e := NewClientIssuer(k, "iss", "aud", time.Hour)
	if _, err := e.Issue(context.Background(), "next-feira"); err != nil {
		t.Fatal(err)
	}
	k.RotateCLI() // o emissor ainda acha que é a v1: assina, vê v2 e refaz uma vez
	em, err := e.Issue(context.Background(), "next-feira")
	if err != nil {
		t.Fatal(err)
	}
	if kid := decodeSegment(t, strings.Split(em.Token, ".")[0])["kid"]; kid != "cli-v2" {
		t.Fatalf("%v", kid)
	}
	if n := k.SignClientCalls.Load(); n != 3 {
		t.Fatalf("1 + 2 assinaturas: %d", n)
	}
}

func TestClientIssuerKMSFailure(t *testing.T) {
	k := porttest.New()
	e := NewClientIssuer(k, "iss", "aud", time.Hour)
	f := error(port.ErrKMSUnavailable)
	k.Failure.Store(&f)
	if _, err := e.Issue(context.Background(), "next-feira"); !errors.Is(err, port.ErrKMSUnavailable) {
		t.Fatal(err)
	}
	if _, err := NewClientValidator(context.Background(), k, "iss", "aud", 0); !errors.Is(err, port.ErrKMSUnavailable) {
		t.Fatal(err)
	}
}

func TestClientValidatorKeysEmptyBeforeLoad(t *testing.T) {
	v := &ClientValidator{}
	if v.Ready() || len(v.ClientKeys()) != 0 {
		t.Fatal("sem chaves carregadas")
	}
}

func TestClientValidatorUpdatePeriodically(t *testing.T) {
	k, _, v, _ := newClientTest(t)
	ctx, cancel := context.WithCancel(context.Background())
	k.RotateCLI()
	done := make(chan struct{})
	go func() { v.UpdatePeriodically(ctx, 5*time.Millisecond); close(done) }()
	deadline := time.Now().Add(2 * time.Second)
	for len(v.ClientKeys()) != 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done
	if len(v.ClientKeys()) != 2 {
		t.Fatalf("a recarga periódica deveria ter trazido a v2: %+v", v.ClientKeys())
	}
}

// O token do app não vale para o ClientValidator e o de cliente não vale para o Validator, nos dois sentidos.
func TestAppAndClientTokensAreIsolated(t *testing.T) {
	k := porttest.New()
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0)
	clock := func() time.Time { return now }
	app := NewIssuer(k, "iss", "tccelta-ingestao", time.Hour)
	app.SetClock(clock)
	cli := NewClientIssuer(k, "iss", "tccelta-analise", time.Hour)
	cli.SetClock(clock)
	appVal, _ := NewValidator(ctx, k, "iss", "tccelta-ingestao", 30*time.Second)
	appVal.SetClock(clock)
	cliVal, _ := NewClientValidator(ctx, k, "iss", "tccelta-analise", 30*time.Second)
	cliVal.SetClock(clock)

	appTok, _ := app.Issue(ctx, dev, 7)
	cliTok, _ := cli.Issue(ctx, "next-feira")
	if _, err := appVal.Validate(ctx, appTok.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := cliVal.Validate(ctx, cliTok.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := cliVal.Validate(ctx, appTok.Token); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("token do app na rota de viagem: %v", err)
	}
	if _, err := appVal.Validate(ctx, cliTok.Token); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("token de cliente na ingestão: %v", err)
	}
	// mesmo com a audiência igual nos dois lados, o kid e as chaves ainda os separam
	sameAud := NewClientIssuer(k, "iss", "tccelta-ingestao", time.Hour)
	sameAud.SetClock(clock)
	tk, _ := sameAud.Issue(ctx, "next-feira")
	if _, err := appVal.Validate(ctx, tk.Token); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("aud igual não basta: %v", err)
	}
	for _, kid := range appVal.keysKIDs() {
		if strings.HasPrefix(kid, ClientKIDPrefix) {
			t.Fatalf("o mapa do app tem chave cli: %s", kid)
		}
	}
	for _, c := range cliVal.ClientKeys() {
		if strings.HasPrefix(c.KID, "jwt-v") {
			t.Fatalf("o mapa de cliente tem chave jwt: %s", c.KID)
		}
	}
}

func (v *Validator) keysKIDs() []string {
	var out []string
	if m := v.keys.Load(); m != nil {
		for k := range *m {
			out = append(out, k)
		}
	}
	return out
}
