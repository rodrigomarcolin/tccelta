package jwt

import (
	"context"
	"crypto/ed25519"
	"errors"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"tccelta/backend/internal/domain/clientauth"
	"tccelta/backend/internal/port"
)

// ClientKIDPrefix é o prefixo do kid dos tokens de cliente ("cli-v<n>"); o do app é "jwt-v<n>".
const ClientKIDPrefix = "cli-v"

// ClientValidator valida os tokens de cliente localmente com as chaves `cli` em memória. É um tipo à parte do Validator:
// os dois mapas de chaves vêm de rotas diferentes do KMS e nunca se misturam, então um token do app não passa aqui
// (kid jwt-v<n>, aud de ingestão) nem um token de cliente passa na ingestão.
type ClientValidator struct {
	keys             atomic.Pointer[map[string]ed25519.PublicKey]
	source           port.ClientSigner
	issuer, audience string
	leeway           time.Duration
	now              func() time.Time
	mu               sync.Mutex
	lastReload       time.Time
}

// NewClientValidator carrega as chaves; falha se o KMS não responder.
func NewClientValidator(ctx context.Context, source port.ClientSigner, issuer, audience string, leeway time.Duration) (*ClientValidator, error) {
	v := &ClientValidator{source: source, issuer: issuer, audience: audience, leeway: leeway, now: time.Now}
	if err := v.reload(ctx); err != nil {
		return nil, err
	}
	return v, nil
}

// SetClock troca o relógio (testes).
func (v *ClientValidator) SetClock(f func() time.Time) { v.now = f }

// Ready: há chaves carregadas.
func (v *ClientValidator) Ready() bool { m := v.keys.Load(); return m != nil && len(*m) > 0 }

// ClientKeys devolve as chaves em memória, ordenadas pelo kid (para o JWKS).
func (v *ClientValidator) ClientKeys() []port.ClientKey {
	m := v.keys.Load()
	if m == nil {
		return nil
	}
	out := make([]port.ClientKey, 0, len(*m))
	for kid, k := range *m {
		out = append(out, port.ClientKey{KID: kid, Key: k})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].KID < out[j].KID })
	return out
}

func (v *ClientValidator) reload(ctx context.Context) error {
	ks, err := v.source.ClientPublicKeys(ctx)
	if err != nil {
		return err
	}
	m := make(map[string]ed25519.PublicKey, len(ks))
	for ver, k := range ks {
		m[ClientKIDPrefix+strconv.Itoa(ver)] = k
	}
	v.keys.Store(&m)
	v.mu.Lock()
	v.lastReload = v.now()
	v.mu.Unlock()
	return nil
}

// reloadByKid recarrega no máximo 1 vez a cada 30 s.
func (v *ClientValidator) reloadByKid(ctx context.Context) {
	v.mu.Lock()
	ok := v.now().Sub(v.lastReload) >= 30*time.Second
	if ok {
		v.lastReload = v.now()
	}
	v.mu.Unlock()
	if ok {
		_ = v.reload(ctx)
	}
}

// UpdatePeriodically recarrega as públicas a cada intervalo (padrão 10 min).
func (v *ClientValidator) UpdatePeriodically(ctx context.Context, each time.Duration) {
	if each <= 0 {
		each = 10 * time.Minute
	}
	t := time.NewTicker(each)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_ = v.reload(ctx)
		}
	}
}

// Validate valida o JWT bruto: alg EdDSA, kid cli-v<n> conhecido, iss, aud, exp obrigatório, sub no formato de client_id e jti UUID.
func (v *ClientValidator) Validate(ctx context.Context, raw string) (port.ClientSession, error) {
	var cl jwt.RegisteredClaims
	triedReload := false
	keyfunc := func(t *jwt.Token) (any, error) {
		kid, ok := t.Header["kid"].(string)
		if !ok || kid == "" {
			return nil, ErrInvalidToken
		}
		if m := v.keys.Load(); m != nil {
			if k, ok := (*m)[kid]; ok {
				return k, nil
			}
		}
		if !triedReload {
			triedReload = true
			v.reloadByKid(ctx)
			if m := v.keys.Load(); m != nil {
				if k, ok := (*m)[kid]; ok {
					return k, nil
				}
			}
		}
		return nil, ErrInvalidToken
	}
	p := jwt.NewParser(
		jwt.WithValidMethods([]string{"EdDSA"}),
		jwt.WithIssuer(v.issuer),
		jwt.WithAudience(v.audience),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(v.leeway),
		jwt.WithTimeFunc(v.now),
	)
	t, err := p.ParseWithClaims(raw, &cl, keyfunc)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) && !errors.Is(err, jwt.ErrTokenSignatureInvalid) {
			return port.ClientSession{}, ErrTokenExpired
		}
		return port.ClientSession{}, ErrInvalidToken
	}
	if !t.Valid || cl.ExpiresAt == nil || !clientauth.ValidID(cl.Subject) {
		return port.ClientSession{}, ErrInvalidToken
	}
	jti, err := uuid.Parse(cl.ID)
	if err != nil {
		return port.ClientSession{}, ErrInvalidToken
	}
	return port.ClientSession{ClientID: cl.Subject, JTI: jti, ExpiresAt: cl.ExpiresAt.Time}, nil
}

var (
	_ port.ClientTokenValidator = (*ClientValidator)(nil)
	_ port.ClientKeySet         = (*ClientValidator)(nil)
)
