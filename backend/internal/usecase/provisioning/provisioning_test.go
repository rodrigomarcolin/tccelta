package provisioning

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"tccelta/backend/internal/adapter/memory"
	"tccelta/backend/internal/domain/dongle"
	"tccelta/backend/internal/domain/protocol"
)

var dev = protocol.DevID{0, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77}

func newTest() (*memory.Database, *Registrar) {
	db := memory.New()
	return db, New(db.Repositories(), db, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestRegisterCreatesDongleAndState(t *testing.T) {
	db, uc := newTest()
	ctx := context.Background()
	created, err := uc.Register(ctx, dongle.Registration{DevID: dev, Profile: "hmac", AuthKey: "ka", RecKey: "kr", Description: "teste"})
	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	d, err := db.Repositories().Dongles.FindByDevID(ctx, dev)
	if err != nil || d.AuthKey != "ka" || d.RecKey != "kr" || d.Profile.Name != "hmac" || !d.Active {
		t.Fatalf("%+v %v", d, err)
	}
	if st := db.State(d.ID); st.ContiguousSeq != 0 {
		t.Fatalf("estado %+v", st)
	}
}

// DevId já cadastrado é pulado, sem alterar o cadastro.
func TestRegisterIsIdempotent(t *testing.T) {
	db, uc := newTest()
	ctx := context.Background()
	if _, err := uc.Register(ctx, dongle.Registration{DevID: dev, Profile: "hmac", AuthKey: "ka", RecKey: "kr"}); err != nil {
		t.Fatal(err)
	}
	created, err := uc.Register(ctx, dongle.Registration{DevID: dev, Profile: "hmac", AuthKey: "outra", RecKey: "outra"})
	if err != nil || created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	if d, _ := db.Repositories().Dongles.FindByDevID(ctx, dev); d.AuthKey != "ka" {
		t.Fatalf("o cadastro foi alterado: %+v", d)
	}
}

func TestRegisterAsymKeepsPublicKey(t *testing.T) {
	db, uc := newTest()
	pub := append([]byte{4}, make([]byte, 64)...)
	if created, err := uc.Register(context.Background(), dongle.Registration{DevID: dev, Profile: "asym", AuthKey: "ka", RecKey: "kr", PublicKey: pub}); err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	if d, _ := db.Repositories().Dongles.FindByDevID(context.Background(), dev); len(d.PublicKey) != 65 {
		t.Fatalf("%+v", d)
	}
}

// Falha no meio da transação não deixa dongle pela metade.
func TestRegisterFailureLeavesNothing(t *testing.T) {
	db, uc := newTest()
	created, err := uc.Register(context.Background(), dongle.Registration{DevID: dev, Profile: "inexistente", AuthKey: "ka", RecKey: "kr"})
	if err == nil || created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	if _, err := db.Repositories().Dongles.FindByDevID(context.Background(), dev); err != dongle.ErrUnknown {
		t.Fatalf("o dongle deveria não existir: %v", err)
	}
}
