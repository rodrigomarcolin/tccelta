// Package session tem o caso de uso de autenticação do Bearer, usado pelo middleware JWT.
package session

import (
	"context"
	"errors"

	"tccelta/backend/internal/domain/dongle"
	"tccelta/backend/internal/port"
)

// Authenticated é a sessão validada e o dongle dono dela.
type Authenticated struct {
	Session port.Session
	Dongle  dongle.Dongle
}

// Authenticate valida o token, resolve o dongle e aplica a regra de acesso.
type Authenticate struct {
	validator port.TokenValidator
	dongles   port.Dongles
	allowNone bool
}

// NewAuthenticate liga o caso de uso às suas portas.
func NewAuthenticate(validator port.TokenValidator, dongles port.Dongles, allowNone bool) *Authenticate {
	return &Authenticate{validator: validator, dongles: dongles, allowNone: allowNone}
}

// Run devolve porta.ErrInvalidToken/ErrTokenExpired para token ruim ou de dongle sem cadastro,
// e dongle.ErrInactive/ErrProfileNotAllowed quando o acesso é negado.
func (c *Authenticate) Run(ctx context.Context, raw string) (Authenticated, error) {
	s, err := c.validator.Validate(ctx, raw)
	if err != nil {
		return Authenticated{}, err
	}
	d, err := c.dongles.FindByDevID(ctx, s.Dev)
	if err != nil {
		if errors.Is(err, dongle.ErrUnknown) {
			return Authenticated{}, port.ErrInvalidToken
		}
		return Authenticated{}, err
	}
	if err := d.VerifyAccess(c.allowNone); err != nil {
		return Authenticated{}, err
	}
	return Authenticated{Session: s, Dongle: d}, nil
}
