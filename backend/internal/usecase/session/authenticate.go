// Package sessao tem o caso de uso de autenticação do Bearer, usado pelo middleware JWT.
package sessao

import (
	"context"
	"errors"

	"tccelta/backend/internal/casodeuso/porta"
	"tccelta/backend/internal/dominio/dongle"
	"tccelta/backend/internal/repositorio"
)

// Autenticada é a sessão validada e o dongle dono dela.
type Autenticada struct {
	Sessao porta.Sessao
	Dongle dongle.Dongle
}

// Autenticar valida o token, resolve o dongle e aplica a regra de acesso.
type Autenticar struct {
	validador    porta.ValidadorToken
	dongles      repositorio.Dongles
	permitirNone bool
}

// NovoAutenticar liga o caso de uso às suas portas.
func NovoAutenticar(validador porta.ValidadorToken, dongles repositorio.Dongles, permitirNone bool) *Autenticar {
	return &Autenticar{validador: validador, dongles: dongles, permitirNone: permitirNone}
}

// Executar devolve porta.ErrTokenInvalido/ErrTokenExpirado para token ruim ou de dongle sem cadastro,
// e dongle.ErrInativo/ErrPerfilNaoPermitido quando o acesso é negado.
func (c *Autenticar) Executar(ctx context.Context, bruto string) (Autenticada, error) {
	s, err := c.validador.Validar(ctx, bruto)
	if err != nil {
		return Autenticada{}, err
	}
	d, err := c.dongles.BuscarPorDevID(ctx, s.Dev)
	if err != nil {
		if errors.Is(err, dongle.ErrDesconhecido) {
			return Autenticada{}, porta.ErrTokenInvalido
		}
		return Autenticada{}, err
	}
	if err := d.VerificarAcesso(c.permitirNone); err != nil {
		return Autenticada{}, err
	}
	return Autenticada{Sessao: s, Dongle: d}, nil
}
