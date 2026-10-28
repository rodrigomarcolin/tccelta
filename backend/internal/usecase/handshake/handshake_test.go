package handshake

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"tccelta/backend/internal/domain/dongle"
	dhs "tccelta/backend/internal/domain/handshake"
	"tccelta/backend/internal/domain/occurrence"
	"tccelta/backend/internal/domain/protocol"
	"tccelta/backend/internal/repository/memory"
	"tccelta/backend/internal/usecase/port"
	"tccelta/backend/internal/usecase/port/porttest"
)

var (
	dev   = protocol.DevID{0, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77}
	kAuth = []byte("0123456789abcdef0123456789abcdef")
	kRec  = []byte("fedcba9876543210fedcba9876543210")
	t0    = time.Unix(1_800_000_000, 0)
)

type fakeToken struct{}

func (fakeToken) Issue(_ context.Context, _ protocol.DevID, cs uint32) (port.IssuedToken, error) {
	return port.IssuedToken{Token: "tk", JTI: uuid.New(), ExpiresAt: t0.Add(time.Hour)}, nil
}
func (fakeToken) Validity() time.Duration { return time.Hour }

type testEnv struct {
	db      *memory.Database
	kms     *porttest.Memory
	clock   *porttest.AdjustableClock
	issue   *IssueChallenge
	respond *RespondChallenge
	params  Params
}

func newTest(t *testing.T) *testEnv {
	t.Helper()
	db := memory.New()
	db.AddDongle(dongle.Dongle{ID: 7, DevID: dev, Profile: protocol.ProfileHMAC, AuthKey: "ka", RecKey: "kr", Active: true})
	k := porttest.New()
	k.AddDongle("ka", "kr", kAuth, kRec)
	rel := &porttest.AdjustableClock{T: t0}
	p := Params{ChallengeValidity: 60 * time.Second, MaxPending: 2, AnchorRTTMax: 2 * time.Second, SuggestedTLat: time.Minute}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	repos := db.Repositories()
	return &testEnv{db: db, kms: k, clock: rel, params: p,
		issue:   NewIssueChallenge(repos, k, porttest.Nop{}, rel.Now, log, p),
		respond: NewRespondChallenge(repos, db, k, fakeToken{}, porttest.Nop{}, rel.Now, log, p)}
}

func macTag(key []byte, typ protocol.Type, payload []byte) []byte {
	m := hmac.New(sha256.New, key)
	m.Write(protocol.TagInput(typ, dev, payload))
	return m.Sum(nil)[:protocol.AuthTagSize]
}

// responder monta a resposta do dongle ao desafio emitido.
func (a *testEnv) response(t *testing.T, d IssuedChallenge, tail, head uint32, bad bool) []byte {
	t.Helper()
	tl, _, err := protocol.ReadTLV(d.TLV)
	if err != nil {
		t.Fatal(err)
	}
	ch, _, err := protocol.DecodeChallenge(tl.V)
	if err != nil {
		t.Fatal(err)
	}
	r := protocol.Response{CS: ch.CS, NS: ch.NS, TailSeq: tail, HeadSeq: head, BootID: 1, TMs: 5000}
	tag := macTag(kAuth, protocol.ResponseType, r.Payload())
	if bad {
		tag[0] ^= 1
	}
	return r.Build(tag)
}

func (a *testEnv) request(t *testing.T) IssuedChallenge {
	t.Helper()
	d, err := a.issue.Run(context.Background(), IssueChallengeInput{DevID: dev})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func (a *testEnv) respondWith(tlv []byte) (HandshakeResult, error) {
	return a.respond.Run(context.Background(), RespondChallengeInput{DevID: dev, S2: a.clock.Now(), TLV: tlv})
}

func TestHappyFlow(t *testing.T) {
	a := newTest(t)
	d := a.request(t)
	// o desafio é assinado com K_auth
	tl, _, _ := protocol.ReadTLV(d.TLV)
	ch, tag, _ := protocol.DecodeChallenge(tl.V)
	if !protocol.CompareTag(macTag(kAuth, protocol.ChallengeType, ch.Payload()), tag) {
		t.Fatal("tag do desafio não confere")
	}
	a.clock.Advance(300 * time.Millisecond)
	res, err := a.respondWith(a.response(t, d, 1, 0, false))
	if err != nil {
		t.Fatal(err)
	}
	if res.Token != "tk" || !res.WithAnchor || res.AckSeq != 0 || res.ValidityS != 3600 || res.SuggestedTLat != time.Minute {
		t.Fatalf("%+v", res)
	}
	est := a.db.State(7)
	if est.CConf != int64(ch.CS) || len(est.LastAck) == 0 {
		t.Fatalf("%+v", est)
	}
	g, _ := a.db.Challenge(7, int64(ch.CS))
	if g.Result != dhs.ResultAccepted || len(a.db.Anchors) != 1 || len(a.db.Notifications) != 1 {
		t.Fatalf("%+v ancoras=%d", g, len(a.db.Anchors))
	}
}

func TestPendingChallengesWithRetryAfter(t *testing.T) {
	a := newTest(t)
	a.request(t)
	a.request(t)
	d, err := a.issue.Run(context.Background(), IssueChallengeInput{DevID: dev})
	if !errors.Is(err, dhs.ErrPendingChallenges) || d.RetryAfter != 20*time.Second {
		t.Fatalf("%v %v", err, d.RetryAfter)
	}
}

func TestInvalidTagMarksChallenge(t *testing.T) {
	a := newTest(t)
	d := a.request(t)
	_, err := a.respondWith(a.response(t, d, 1, 0, true))
	if !errors.Is(err, dhs.ErrInvalidTag) {
		t.Fatal(err)
	}
	if g, _ := a.db.Challenge(7, 1); g.Result != "invalid_tag" {
		t.Fatalf("%+v", g)
	}
	if a.db.State(7).CConf != 0 {
		t.Fatal("estado não pode mudar")
	}
}

func TestStaleCS(t *testing.T) {
	a := newTest(t)
	a.db.SetState(7, dongle.State{CConf: 100})
	d := a.request(t) // c_S = 1 <= c_conf
	if _, err := a.respondWith(a.response(t, d, 1, 0, false)); !errors.Is(err, dhs.ErrStaleCS) {
		t.Fatal(err)
	}
}

func TestChallengeUsedOnce(t *testing.T) {
	a := newTest(t)
	d := a.request(t)
	r := a.response(t, d, 1, 0, false)
	if _, err := a.respondWith(r); err != nil {
		t.Fatal(err)
	}
	if _, err := a.respondWith(r); !errors.Is(err, dhs.ErrInvalidChallenge) {
		t.Fatal(err)
	}
}

func TestExpiredChallenge(t *testing.T) {
	a := newTest(t)
	d := a.request(t)
	a.clock.Advance(2 * time.Minute)
	if _, err := a.respondWith(a.response(t, d, 1, 0, false)); !errors.Is(err, dhs.ErrInvalidChallenge) {
		t.Fatal(err)
	}
}

func TestInconsistentTailFinalizesWithoutChangingState(t *testing.T) {
	a := newTest(t)
	d := a.request(t)
	_, err := a.respondWith(a.response(t, d, 10, 3, false)) // tail > head+1
	if !errors.Is(err, dhs.ErrInconsistentResponse) {
		t.Fatal(err)
	}
	g, _ := a.db.Challenge(7, 1)
	if g.Result != dhs.ResultInconsistent || a.db.State(7).CConf != 0 {
		t.Fatalf("%+v %+v", g, a.db.State(7))
	}
}

func TestOverflowRecordsGap(t *testing.T) {
	a := newTest(t)
	a.db.SetState(7, dongle.State{ContiguousSeq: 5, HeadSeq: 5})
	d := a.request(t)
	res, err := a.respondWith(a.response(t, d, 20, 30, false))
	if err != nil {
		t.Fatal(err)
	}
	if res.AckSeq != 19 || len(a.db.Gaps) != 1 {
		t.Fatalf("%+v %+v", res, a.db.Gaps)
	}
	l := a.db.Gaps[0]
	if l.Cause != occurrence.CauseOverflow || l.From != 6 || l.To != 19 || l.CS == nil || *l.CS != 1 {
		t.Fatalf("%+v", l)
	}
}

func TestAnchorHighRTT(t *testing.T) {
	a := newTest(t)
	d := a.request(t)
	a.clock.Advance(5 * time.Second) // acima de AnchorRTTMax
	res, err := a.respondWith(a.response(t, d, 1, 0, false))
	if err != nil {
		t.Fatal(err)
	}
	if res.WithAnchor || len(a.db.Anchors) != 0 {
		t.Fatalf("%+v", res)
	}
	tl, _, _ := protocol.ReadTLV(res.AckTLV)
	ack, _, _ := protocol.DecodeAck(tl.V)
	if ack.Flags&protocol.FlagAckAnchorFailed == 0 {
		t.Fatal("ACK deveria sinalizar âncora falhou")
	}
	if g, _ := a.db.Challenge(7, 1); g.Result != dhs.ResultAcceptedNoAnchor {
		t.Fatalf("%+v", g)
	}
}

func TestAccess(t *testing.T) {
	a := newTest(t)
	if _, err := a.issue.Run(context.Background(), IssueChallengeInput{DevID: protocol.DevID{9}}); !errors.Is(err, dongle.ErrUnknown) {
		t.Fatal(err)
	}
	a.db.AddDongle(dongle.Dongle{ID: 8, DevID: protocol.DevID{8}, Profile: protocol.ProfileNone, Active: true})
	if _, err := a.issue.Run(context.Background(), IssueChallengeInput{DevID: protocol.DevID{8}}); !errors.Is(err, dongle.ErrProfileNotAllowed) {
		t.Fatal(err)
	}
	a.db.AddDongle(dongle.Dongle{ID: 9, DevID: protocol.DevID{9}, Profile: protocol.ProfileHMAC, Active: false})
	if _, err := a.issue.Run(context.Background(), IssueChallengeInput{DevID: protocol.DevID{9}}); !errors.Is(err, dongle.ErrInactive) {
		t.Fatal(err)
	}
}

func TestKMSUnavailable(t *testing.T) {
	a := newTest(t)
	var f error = port.ErrKMSUnavailable
	a.kms.Failure.Store(&f)
	if _, err := a.issue.Run(context.Background(), IssueChallengeInput{DevID: dev}); !errors.Is(err, port.ErrKMSUnavailable) {
		t.Fatal(err)
	}
}
