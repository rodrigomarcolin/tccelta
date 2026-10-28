package session

import (
	"context"
	"errors"
	"testing"

	"tccelta/backend/internal/domain/dongle"
	"tccelta/backend/internal/domain/protocol"
	"tccelta/backend/internal/repository/memory"
	"tccelta/backend/internal/usecase/port"
)

type fakeValidator struct {
	s   port.Session
	err error
}

func (v fakeValidator) Validate(context.Context, string) (port.Session, error) { return v.s, v.err }

func TestAuthenticate(t *testing.T) {
	active := protocol.DevID{1}
	inactive := protocol.DevID{2}
	withoutProfile := protocol.DevID{3}
	db := memory.New()
	db.AddDongle(dongle.Dongle{ID: 1, DevID: active, Profile: protocol.ProfileHMAC, Active: true})
	db.AddDongle(dongle.Dongle{ID: 2, DevID: inactive, Profile: protocol.ProfileHMAC})
	db.AddDongle(dongle.Dongle{ID: 3, DevID: withoutProfile, Profile: protocol.ProfileNone, Active: true})
	repos := db.Repositories()
	usecases := []struct {
		name string
		val  fakeValidator
		none bool
		err  error
	}{
		{"ok", fakeValidator{s: port.Session{Dev: active, CS: 1}}, false, nil},
		{"token inválido", fakeValidator{err: port.ErrInvalidToken}, false, port.ErrInvalidToken},
		{"token expirado", fakeValidator{err: port.ErrTokenExpired}, false, port.ErrTokenExpired},
		{"dongle sem cadastro vira token inválido", fakeValidator{s: port.Session{Dev: protocol.DevID{9}}}, false, port.ErrInvalidToken},
		{"dongle inativo", fakeValidator{s: port.Session{Dev: inactive}}, false, dongle.ErrInactive},
		{"perfil none recusado", fakeValidator{s: port.Session{Dev: withoutProfile}}, false, dongle.ErrProfileNotAllowed},
		{"perfil none permitido", fakeValidator{s: port.Session{Dev: withoutProfile, CS: 1}}, true, nil},
	}
	for _, c := range usecases {
		a, err := NewAuthenticate(c.val, repos.Dongles, c.none).Run(context.Background(), "x")
		if !errors.Is(err, c.err) && err != c.err {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if err == nil && a.Dongle.DevID != c.val.s.Dev {
			t.Errorf("%s: dongle %+v", c.name, a.Dongle)
		}
	}
}
