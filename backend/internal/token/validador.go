package token

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

	"tccelta/backend/internal/kms"
	"tccelta/backend/internal/protocolo"
)

// Sessao é o que o JWT validado autoriza.
type Sessao struct {
	Dev      protocolo.DevID
	CS       uint32
	JTI      uuid.UUID
	ExpiraEm time.Time
}

type claims struct {
	jwt.RegisteredClaims
	CS json.Number `json:"cs"`
}

// Validador valida os tokens localmente com as chaves públicas em memória.
type Validador struct {
	chaves             atomic.Pointer[map[string]ed25519.PublicKey]
	fonte              kms.AssinadorJWT
	emissor, audiencia string
	folga              time.Duration
	agora              func() time.Time
	mu                 sync.Mutex
	ultimaRecarga      time.Time
}

// NovoValidador carrega as chaves; falha se o Vault não responder no boot.
func NovoValidador(ctx context.Context, fonte kms.AssinadorJWT, emissor, audiencia string, folga time.Duration) (*Validador, error) {
	v := &Validador{fonte: fonte, emissor: emissor, audiencia: audiencia, folga: folga, agora: time.Now}
	if err := v.recarregar(ctx); err != nil {
		return nil, err
	}
	return v, nil
}

// DefinirRelogio troca o relógio (testes).
func (v *Validador) DefinirRelogio(f func() time.Time) { v.agora = f }

// Pronto: há chaves carregadas.
func (v *Validador) Pronto() bool { m := v.chaves.Load(); return m != nil && len(*m) > 0 }

func (v *Validador) recarregar(ctx context.Context) error {
	ks, err := v.fonte.ChavesPublicas(ctx)
	if err != nil {
		return err
	}
	m := make(map[string]ed25519.PublicKey, len(ks))
	for ver, k := range ks {
		m["jwt-v"+strconv.Itoa(ver)] = k
	}
	v.chaves.Store(&m)
	v.mu.Lock()
	v.ultimaRecarga = v.agora()
	v.mu.Unlock()
	return nil
}

// recarregarPorKid recarrega no máximo 1 vez a cada 30 s.
func (v *Validador) recarregarPorKid(ctx context.Context) {
	v.mu.Lock()
	ok := v.agora().Sub(v.ultimaRecarga) >= 30*time.Second
	if ok {
		v.ultimaRecarga = v.agora()
	}
	v.mu.Unlock()
	if ok {
		_ = v.recarregar(ctx)
	}
}

// AtualizarPeriodicamente recarrega as públicas a cada intervalo (padrão 10 min).
func (v *Validador) AtualizarPeriodicamente(ctx context.Context, cada time.Duration) {
	if cada <= 0 {
		cada = 10 * time.Minute
	}
	t := time.NewTicker(cada)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_ = v.recarregar(ctx)
		}
	}
}

// Validar valida o JWT bruto.
func (v *Validador) Validar(ctx context.Context, bruto string) (Sessao, error) {
	var cl claims
	tentouRecarregar := false
	keyfunc := func(t *jwt.Token) (any, error) {
		kid, ok := t.Header["kid"].(string)
		if !ok || kid == "" {
			return nil, ErrTokenInvalido
		}
		m := v.chaves.Load()
		if m != nil {
			if k, ok := (*m)[kid]; ok {
				return k, nil
			}
		}
		if !tentouRecarregar {
			tentouRecarregar = true
			v.recarregarPorKid(ctx)
			if m = v.chaves.Load(); m != nil {
				if k, ok := (*m)[kid]; ok {
					return k, nil
				}
			}
		}
		return nil, ErrTokenInvalido
	}
	p := jwt.NewParser(
		jwt.WithValidMethods([]string{"EdDSA"}),
		jwt.WithIssuer(v.emissor),
		jwt.WithAudience(v.audiencia),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(v.folga),
		jwt.WithJSONNumber(),
		jwt.WithTimeFunc(v.agora),
	)
	t, err := p.ParseWithClaims(bruto, &cl, keyfunc)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) && !errors.Is(err, jwt.ErrTokenSignatureInvalid) {
			return Sessao{}, ErrTokenExpirado
		}
		return Sessao{}, ErrTokenInvalido
	}
	if !t.Valid || cl.ExpiresAt == nil {
		return Sessao{}, ErrTokenInvalido
	}
	if len(cl.Subject) != 16 {
		return Sessao{}, ErrTokenInvalido
	}
	dev, err := protocolo.DevIDDeHex(strings.ToLower(cl.Subject))
	if err != nil {
		return Sessao{}, ErrTokenInvalido
	}
	cs, err := strconv.ParseUint(cl.CS.String(), 10, 32)
	if err != nil || cs < 1 {
		return Sessao{}, ErrTokenInvalido
	}
	jti, err := uuid.Parse(cl.ID)
	if err != nil {
		return Sessao{}, ErrTokenInvalido
	}
	return Sessao{Dev: dev, CS: uint32(cs), JTI: jti, ExpiraEm: cl.ExpiresAt.Time}, nil
}
