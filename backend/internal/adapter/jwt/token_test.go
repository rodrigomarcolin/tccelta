package jwt

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

	"tccelta/backend/internal/domain/protocol"
	"tccelta/backend/internal/port/porttest"
)

var dev, _ = protocol.DevIDFromHex("0011223344556677")

func newTest(t *testing.T) (*porttest.Memory, *Issuer, *Validator, *time.Time) {
	t.Helper()
	k := porttest.New()
	now := time.Unix(1_800_000_000, 0)
	e := NewIssuer(k, "iss", "aud", time.Hour)
	e.SetClock(func() time.Time { return now })
	v, err := NewValidator(context.Background(), k, "iss", "aud", 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	v.SetClock(func() time.Time { return now })
	return k, e, v, &now
}

func TestRoundTrip(t *testing.T) {
	_, e, v, _ := newTest(t)
	em, err := e.Issue(context.Background(), dev, 1_700_000_007)
	if err != nil {
		t.Fatal(err)
	}
	s, err := v.Validate(context.Background(), em.Token)
	if err != nil {
		t.Fatal(err)
	}
	if s.Dev != dev || s.CS != 1_700_000_007 || s.JTI != em.JTI {
		t.Fatalf("%+v", s)
	}
}

func stitch(hd, cl map[string]any, sign func(string) string) string {
	h, _ := json.Marshal(hd)
	c, _ := json.Marshal(cl)
	a := base64.RawURLEncoding.EncodeToString(h) + "." + base64.RawURLEncoding.EncodeToString(c)
	return a + "." + sign(a)
}

func TestNegatives(t *testing.T) {
	k, e, v, now := newTest(t)
	ctx := context.Background()
	ok, _ := e.Issue(ctx, dev, 5)
	base := func() map[string]any {
		return map[string]any{"iss": "iss", "aud": "aud", "sub": dev.Hex(), "jti": "6f1c1c1e-1d7e-4a39-9d1b-0a8b1b0f0001",
			"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(), "cs": 5}
	}
	hdr := map[string]any{"alg": "EdDSA", "typ": "JWT", "kid": "jwt-v1"}
	withoutSign := func(string) string { return "AAAA" }
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	wrongSignature := func(a string) string { return base64.RawURLEncoding.EncodeToString(ed25519.Sign(priv, []byte(a))) }

	pubs, _ := k.PublicKeys(ctx)
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
	clLarge := base()
	clLarge["cs"] = 4294967296
	clNoExp := base()
	delete(clNoExp, "exp")
	clIss := base()
	clIss["iss"] = "outro"
	clAud := base()
	clAud["aud"] = "outro"

	usecases := map[string]string{
		"alg none":    stitch(map[string]any{"alg": "none", "typ": "JWT", "kid": "jwt-v1"}, base(), func(string) string { return "" }),
		"hs256":       stitch(map[string]any{"alg": "HS256", "typ": "JWT", "kid": "jwt-v1"}, base(), hs),
		"kid desc":    stitch(map[string]any{"alg": "EdDSA", "typ": "JWT", "kid": "jwt-v9"}, base(), wrongSignature),
		"kid num":     stitch(map[string]any{"alg": "EdDSA", "typ": "JWT", "kid": 1}, base(), wrongSignature),
		"iss":         stitch(hdr, clIss, wrongSignature),
		"aud":         stitch(hdr, clAud, wrongSignature),
		"sub curto":   stitch(hdr, cl, wrongSignature),
		"cs frac":     stitch(hdr, clFrac, wrongSignature),
		"cs grande":   stitch(hdr, clLarge, wrongSignature),
		"sem exp":     stitch(hdr, clNoExp, wrongSignature),
		"sem assin":   stitch(hdr, base(), withoutSign),
		"lixo":        "abc",
		"bit trocado": ok.Token[:len(ok.Token)-2] + string(rune(ok.Token[len(ok.Token)-2]^1)) + ok.Token[len(ok.Token)-1:],
	}
	for name, tk := range usecases {
		if _, err := v.Validate(ctx, tk); err != ErrInvalidToken {
			t.Errorf("%s: esperava invalido, veio %v", name, err)
		}
	}
	_ = now
}

func TestExpirationWithLeeway(t *testing.T) {
	_, e, v, now := newTest(t)
	em, _ := e.Issue(context.Background(), dev, 5)
	*now = em.ExpiresAt.Add(29 * time.Second)
	if _, err := v.Validate(context.Background(), em.Token); err != nil {
		t.Fatalf("29 s dentro da folga: %v", err)
	}
	*now = em.ExpiresAt.Add(31 * time.Second)
	if _, err := v.Validate(context.Background(), em.Token); err != ErrTokenExpired {
		t.Fatalf("31 s: %v", err)
	}
}

func TestRotation(t *testing.T) {
	k, e, v, now := newTest(t)
	ctx := context.Background()
	v1, _ := e.Issue(ctx, dev, 5)
	k.RotateJWT()
	v2, err := e.Issue(ctx, dev, 6)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(v2.Token, ".") {
		t.Fatal("token")
	}
	*now = now.Add(time.Minute) // passa o limite de 30 s da recarga por kid
	if _, err := v.Validate(ctx, v2.Token); err != nil {
		t.Fatalf("kid novo: %v", err)
	}
	if _, err := v.Validate(ctx, v1.Token); err != nil {
		t.Fatalf("kid antigo ainda valido: %v", err)
	}
	k.PurgeJWT(2)
	if err := v.reload(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Validate(ctx, v1.Token); err != ErrInvalidToken {
		t.Fatalf("v1 expurgada: %v", err)
	}
}
