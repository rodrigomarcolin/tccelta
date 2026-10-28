package jwt

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"tccelta/backend/internal/domain/protocol"
	"tccelta/backend/internal/port"
)

type claims struct {
	jwt.RegisteredClaims
	CS json.Number `json:"cs"`
}

// Validator valida os tokens localmente com as chaves públicas em memória.
type Validator struct {
	keys             atomic.Pointer[map[string]ed25519.PublicKey]
	source           port.JWTSigner
	issuer, audience string
	leeway           time.Duration
	now              func() time.Time
	mu               sync.Mutex
	lastReload       time.Time
}

// NewValidator carrega as chaves; falha se o Vault não responder no boot.
func NewValidator(ctx context.Context, source port.JWTSigner, issuer, audience string, leeway time.Duration) (*Validator, error) {
	v := &Validator{source: source, issuer: issuer, audience: audience, leeway: leeway, now: time.Now}
	if err := v.reload(ctx); err != nil {
		return nil, err
	}
	return v, nil
}

// SetClock troca o relógio (testes).
func (v *Validator) SetClock(f func() time.Time) { v.now = f }

// Ready: há chaves carregadas.
func (v *Validator) Ready() bool { m := v.keys.Load(); return m != nil && len(*m) > 0 }

func (v *Validator) reload(ctx context.Context) error {
	ks, err := v.source.PublicKeys(ctx)
	if err != nil {
		return err
	}
	m := make(map[string]ed25519.PublicKey, len(ks))
	for verify, k := range ks {
		m["jwt-v"+strconv.Itoa(verify)] = k
	}
	v.keys.Store(&m)
	v.mu.Lock()
	v.lastReload = v.now()
	v.mu.Unlock()
	return nil
}

// reloadByKid recarrega no máximo 1 vez a cada 30 s.
func (v *Validator) reloadByKid(ctx context.Context) {
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
func (v *Validator) UpdatePeriodically(ctx context.Context, each time.Duration) {
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

// Validate valida o JWT bruto.
func (v *Validator) Validate(ctx context.Context, raw string) (port.Session, error) {
	var cl claims
	triedReload := false
	keyfunc := func(t *jwt.Token) (any, error) {
		kid, ok := t.Header["kid"].(string)
		if !ok || kid == "" {
			return nil, ErrInvalidToken
		}
		m := v.keys.Load()
		if m != nil {
			if k, ok := (*m)[kid]; ok {
				return k, nil
			}
		}
		if !triedReload {
			triedReload = true
			v.reloadByKid(ctx)
			if m = v.keys.Load(); m != nil {
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
		jwt.WithJSONNumber(),
		jwt.WithTimeFunc(v.now),
	)
	t, err := p.ParseWithClaims(raw, &cl, keyfunc)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) && !errors.Is(err, jwt.ErrTokenSignatureInvalid) {
			return port.Session{}, ErrTokenExpired
		}
		return port.Session{}, ErrInvalidToken
	}
	if !t.Valid || cl.ExpiresAt == nil {
		return port.Session{}, ErrInvalidToken
	}
	if len(cl.Subject) != 16 {
		return port.Session{}, ErrInvalidToken
	}
	dev, err := protocol.DevIDFromHex(strings.ToLower(cl.Subject))
	if err != nil {
		return port.Session{}, ErrInvalidToken
	}
	cs, err := strconv.ParseUint(cl.CS.String(), 10, 32)
	if err != nil || cs < 1 {
		return port.Session{}, ErrInvalidToken
	}
	jti, err := uuid.Parse(cl.ID)
	if err != nil {
		return port.Session{}, ErrInvalidToken
	}
	return port.Session{Dev: dev, CS: uint32(cs), JTI: jti, ExpiresAt: cl.ExpiresAt.Time}, nil
}

var _ port.TokenIssuer = (*Issuer)(nil)
var _ port.TokenValidator = (*Validator)(nil)
