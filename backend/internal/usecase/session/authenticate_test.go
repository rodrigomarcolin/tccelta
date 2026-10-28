package sessao

import (
	"context"
	"errors"
	"testing"

	"tccelta/backend/internal/casodeuso/porta"
	"tccelta/backend/internal/dominio/dongle"
	"tccelta/backend/internal/dominio/protocolo"
	"tccelta/backend/internal/repositorio/memoria"
)

type validadorFake struct {
	s   porta.Sessao
	err error
}

func (v validadorFake) Validar(context.Context, string) (porta.Sessao, error) { return v.s, v.err }

func TestAutenticar(t *testing.T) {
	ativo := protocolo.DevID{1}
	inativo := protocolo.DevID{2}
	semPerfil := protocolo.DevID{3}
	bd := memoria.Novo()
	bd.AdicionarDongle(dongle.Dongle{ID: 1, DevID: ativo, Perfil: protocolo.PerfilHMAC, Ativo: true})
	bd.AdicionarDongle(dongle.Dongle{ID: 2, DevID: inativo, Perfil: protocolo.PerfilHMAC})
	bd.AdicionarDongle(dongle.Dongle{ID: 3, DevID: semPerfil, Perfil: protocolo.PerfilNone, Ativo: true})
	repos := bd.Repositorios()
	casos := []struct {
		nome string
		val  validadorFake
		none bool
		erro error
	}{
		{"ok", validadorFake{s: porta.Sessao{Dev: ativo, CS: 1}}, false, nil},
		{"token inválido", validadorFake{err: porta.ErrTokenInvalido}, false, porta.ErrTokenInvalido},
		{"token expirado", validadorFake{err: porta.ErrTokenExpirado}, false, porta.ErrTokenExpirado},
		{"dongle sem cadastro vira token inválido", validadorFake{s: porta.Sessao{Dev: protocolo.DevID{9}}}, false, porta.ErrTokenInvalido},
		{"dongle inativo", validadorFake{s: porta.Sessao{Dev: inativo}}, false, dongle.ErrInativo},
		{"perfil none recusado", validadorFake{s: porta.Sessao{Dev: semPerfil}}, false, dongle.ErrPerfilNaoPermitido},
		{"perfil none permitido", validadorFake{s: porta.Sessao{Dev: semPerfil, CS: 1}}, true, nil},
	}
	for _, c := range casos {
		a, err := NovoAutenticar(c.val, repos.Dongles, c.none).Executar(context.Background(), "x")
		if !errors.Is(err, c.erro) && err != c.erro {
			t.Errorf("%s: %v", c.nome, err)
			continue
		}
		if err == nil && a.Dongle.DevID != c.val.s.Dev {
			t.Errorf("%s: dongle %+v", c.nome, a.Dongle)
		}
	}
}
