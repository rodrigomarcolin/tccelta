package clienttoken

import (
	"context"
	"crypto/ed25519"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"tccelta/backend/internal/adapter/authblock"
	"tccelta/backend/internal/adapter/jwt"
	"tccelta/backend/internal/domain/clientauth"
	"tccelta/backend/internal/port"
	"tccelta/backend/internal/port/porttest"
)

type metrics struct {
	porttest.Nop
	blocks []string
}

func (m *metrics) Block(scope string) { m.blocks = append(m.blocks, scope) }

const secret = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFG"

type env struct {
	uc  *IssueToken
	kms *porttest.Memory
	met *metrics
	now *time.Time
}

func newEnv(t *testing.T) *env {
	t.Helper()
	k := porttest.New()
	k.AddClient("next-dev", secret)
	now := time.Unix(1_800_000_000, 0)
	pol := clientauth.Policy{Tolerance: 5, Initial: 30 * time.Second, Factor: 2, Cap: 15 * time.Minute, Forget: 30 * time.Minute}
	ip := pol
	ip.Tolerance = 20
	blocker := authblock.New(nil, 0, authblock.Policies{Pair: pol, IP: ip}, nil)
	met := &metrics{}
	iss := jwt.NewClientIssuer(k, "iss", "aud", 30*time.Minute)
	iss.SetClock(func() time.Time { return now })
	return &env{uc: NewIssueToken(k, iss, blocker, met, slog.New(slog.NewTextHandler(io.Discard, nil))), kms: k, met: met, now: &now}
}

func creds(id, sec, ip string) Credentials {
	return Credentials{Present: true, ClientID: id, Secret: sec, IP: ip}
}

func TestIssueSuccess(t *testing.T) {
	e := newEnv(t)
	tok, err := e.uc.Run(context.Background(), creds("next-dev", secret, "1.1.1.1"))
	if err != nil || tok.AccessToken == "" || tok.ExpiresIn != 30*time.Minute {
		t.Fatalf("%+v %v", tok, err)
	}
	if e.kms.VerifyClientCalls.Load() != 1 || e.kms.SignClientCalls.Load() != 1 {
		t.Fatal("uma verificação e uma assinatura")
	}
}

// Segredo errado, cliente desconhecido e revogado dão o mesmo erro, cada um com uma chamada ao KMS.
func TestUniformFailureAndCost(t *testing.T) {
	e := newEnv(t)
	e.kms.AddClient("velho-dev", secret)
	e.kms.RevokeClient("velho-dev")
	for name, c := range map[string]Credentials{
		"segredo errado": creds("next-dev", secret+"x", "1.1.1.1"),
		"desconhecido":   creds("outro-dev", secret, "1.1.1.2"),
		"revogado":       creds("velho-dev", secret, "1.1.1.3"),
	} {
		before := e.kms.VerifyClientCalls.Load()
		if _, err := e.uc.Run(context.Background(), c); !errors.Is(err, clientauth.ErrInvalidClient) {
			t.Errorf("%s: %v", name, err)
		}
		if got := e.kms.VerifyClientCalls.Load() - before; got != 1 {
			t.Errorf("%s: %d chamadas ao KMS, esperava 1 (custo uniforme)", name, got)
		}
	}
	if e.kms.SignClientCalls.Load() != 0 {
		t.Fatal("falha não assina")
	}
}

func TestMalformedCredentialsCountOnlyIP(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	for _, c := range []Credentials{
		{Present: false, IP: "2.2.2.2"},
		creds("Next Dev", secret, "2.2.2.2"),
		creds("next-dev", "", "2.2.2.2"),
	} {
		if _, err := e.uc.Run(ctx, c); !errors.Is(err, clientauth.ErrInvalidClient) {
			t.Fatalf("%+v: %v", c, err)
		}
	}
	if e.kms.VerifyClientCalls.Load() != 0 {
		t.Fatal("credencial malformada nem chega ao KMS")
	}
	// o par (next-dev, 2.2.2.2) não foi contado: 5 falhas reais ainda não bloqueiam
	for i := 0; i < 5; i++ {
		_, _ = e.uc.Run(ctx, creds("next-dev", "errado", "2.2.2.2"))
	}
	if _, err := e.uc.Run(ctx, creds("next-dev", secret, "2.2.2.2")); err != nil {
		t.Fatalf("o par tem 5 falhas, dentro da tolerância: %v", err)
	}
}

func TestProgressiveBlockSkipsKMS(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	for i := 1; i <= 6; i++ {
		if _, err := e.uc.Run(ctx, creds("next-dev", "errado", "3.3.3.3")); !errors.Is(err, clientauth.ErrInvalidClient) {
			t.Fatalf("falha %d: %v", i, err)
		}
	}
	calls := e.kms.VerifyClientCalls.Load()
	_, err := e.uc.Run(ctx, creds("next-dev", secret, "3.3.3.3")) // até a credencial certa fica bloqueada
	var be clientauth.BlockedError
	if !errors.Is(err, clientauth.ErrBlocked) || !errors.As(err, &be) || be.RetryAfter < time.Second || be.RetryAfter > 30*time.Second {
		t.Fatalf("7ª tentativa: %v", err)
	}
	for i := 0; i < 3; i++ {
		_, _ = e.uc.Run(ctx, creds("next-dev", "errado", "3.3.3.3"))
	}
	if e.kms.VerifyClientCalls.Load() != calls {
		t.Fatal("durante o bloqueio o KMS não recebe chamada")
	}
	if len(e.met.blocks) != 1 || e.met.blocks[0] != "pair" {
		t.Fatalf("um bloqueio novo do par: %v", e.met.blocks)
	}
	// outro IP segue livre
	if _, err := e.uc.Run(ctx, creds("next-dev", secret, "4.4.4.4")); err != nil {
		t.Fatalf("(a, IP2): %v", err)
	}
}

func TestSuccessResetsPairKeepsIP(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		_, _ = e.uc.Run(ctx, creds("next-dev", "errado", "5.5.5.5"))
	}
	if _, err := e.uc.Run(ctx, creds("next-dev", secret, "5.5.5.5")); err != nil {
		t.Fatal(err)
	}
	// o par recomeçou: mais 5 falhas não bloqueiam
	for i := 0; i < 5; i++ {
		_, _ = e.uc.Run(ctx, creds("next-dev", "errado", "5.5.5.5"))
	}
	if _, err := e.uc.Run(ctx, creds("next-dev", secret, "5.5.5.5")); err != nil {
		t.Fatalf("o sucesso zerou o par: %v", err)
	}
	// o IP acumulou 10 falhas; mais 11 de client_ids diferentes o bloqueiam (21 no total)
	for i := 0; i < 10; i++ {
		_, _ = e.uc.Run(ctx, creds("cli-"+string(rune('a'+i))+"x", "errado", "5.5.5.5"))
	}
	if _, err := e.uc.Run(ctx, creds("next-dev", secret, "5.5.5.5")); err != nil {
		t.Fatalf("20 falhas do IP ainda dentro da tolerância: %v", err)
	}
	_, _ = e.uc.Run(ctx, creds("cli-zzz", "errado", "5.5.5.5"))
	if _, err := e.uc.Run(ctx, creds("next-dev", secret, "5.5.5.5")); !errors.Is(err, clientauth.ErrBlocked) {
		t.Fatalf("o contador do IP sobreviveu ao sucesso: %v", err)
	}
	if got := e.met.blocks; len(got) == 0 || got[len(got)-1] != "ip" {
		t.Fatalf("%v", got)
	}
}

func TestKMSUnavailableDoesNotCount(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	f := error(port.ErrKMSUnavailable)
	e.kms.Failure.Store(&f)
	for i := 0; i < 10; i++ {
		if _, err := e.uc.Run(ctx, creds("next-dev", secret, "6.6.6.6")); !errors.Is(err, port.ErrKMSUnavailable) {
			t.Fatal(err)
		}
	}
	var none error
	e.kms.Failure.Store(&none)
	if _, err := e.uc.Run(ctx, creds("next-dev", secret, "6.6.6.6")); err != nil {
		t.Fatalf("503 não conta como falha: %v", err)
	}
}

func TestSigningFailureSurfaces(t *testing.T) {
	e := newEnv(t)
	// a verificação passa; a assinatura falha porque o KMS cai no meio
	k := &flaky{Memory: e.kms}
	iss := jwt.NewClientIssuer(k, "iss", "aud", time.Hour)
	uc := NewIssueToken(k, iss, authblock.New(nil, 0, authblock.Policies{
		Pair: clientauth.Policy{Tolerance: 5, Initial: time.Second, Factor: 2, Cap: time.Minute, Forget: time.Hour},
		IP:   clientauth.Policy{Tolerance: 5, Initial: time.Second, Factor: 2, Cap: time.Minute, Forget: time.Hour}}, nil), nil,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if _, err := uc.Run(context.Background(), creds("next-dev", secret, "7.7.7.7")); !errors.Is(err, port.ErrKMSUnavailable) {
		t.Fatal(err)
	}
}

// flaky verifica, mas não assina nem lista as chaves.
type flaky struct{ *porttest.Memory }

func (f *flaky) SignClient(context.Context, []byte) ([]byte, int, error) {
	return nil, 0, port.ErrKMSUnavailable
}

func (f *flaky) ClientPublicKeys(context.Context) (map[int]ed25519.PublicKey, error) {
	return nil, port.ErrKMSUnavailable
}

func TestAuthenticateAndGetKeys(t *testing.T) {
	k := porttest.New()
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0)
	iss := jwt.NewClientIssuer(k, "iss", "aud", time.Hour)
	iss.SetClock(func() time.Time { return now })
	val, err := jwt.NewClientValidator(ctx, k, "iss", "aud", 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	val.SetClock(func() time.Time { return now })
	tok, _ := iss.Issue(ctx, "next-dev")
	s, err := NewAuthenticate(val).Run(ctx, tok.Token)
	if err != nil || s.ClientID != "next-dev" {
		t.Fatalf("%+v %v", s, err)
	}
	if _, err := NewAuthenticate(val).Run(ctx, "lixo"); !errors.Is(err, port.ErrInvalidToken) {
		t.Fatal(err)
	}
	ks, err := NewGetKeys(val).Run(ctx)
	if err != nil || len(ks) != 1 || ks[0].KID != "cli-v1" {
		t.Fatalf("%+v %v", ks, err)
	}
	if _, err := NewGetKeys(&jwt.ClientValidator{}).Run(ctx); !errors.Is(err, ErrNoKeys) {
		t.Fatalf("sem chaves: %v", err)
	}
}
