// Package handshake tem os casos de uso do desafio e da resposta (spec 04).
package handshake

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"log/slog"
	"time"

	dhs "tccelta/backend/internal/domain/handshake"
	"tccelta/backend/internal/domain/protocol"
	"tccelta/backend/internal/port"
)

// Params são os ajustes do handshake.
type Params struct {
	ChallengeValidity time.Duration
	MaxPending        int
	AnchorRTTMax      time.Duration
	SuggestedTLat     time.Duration
	AllowNoneProfile  bool
}

// IssueChallengeInput identifica o dongle que pede o desafio.
type IssueChallengeInput struct{ DevID protocol.DevID }

// IssuedChallenge é a saída de IssueChallenge.
type IssuedChallenge struct {
	TLV       []byte
	ExpiresAt time.Time
	// RetryAfter é preenchido junto com dhs.ErrPendingChallenges.
	RetryAfter time.Duration
}

// IssueChallenge gera o Desafio 0x01 assinado com K_auth (POST /v1/dongles/{id}/desafio).
type IssueChallenge struct {
	repos port.Repositories
	kms   port.Authenticator
	met   port.MetricsHandshake
	now   port.Clock
	rand  io.Reader
	log   *slog.Logger
	p     Params
}

// NewIssueChallenge liga o caso de uso às suas portas.
func NewIssueChallenge(repos port.Repositories, kms port.Authenticator, met port.MetricsHandshake,
	now port.Clock, log *slog.Logger, p Params) *IssueChallenge {
	return &IssueChallenge{repos: repos, kms: kms, met: met, now: now, rand: rand.Reader, log: log, p: p}
}

func tag16(mac []byte) []byte { return mac[:protocol.AuthTagSize] }

// Run emite o desafio.
func (c *IssueChallenge) Run(ctx context.Context, e IssueChallengeInput) (IssuedChallenge, error) {
	d, err := c.repos.Dongles.FindByDevID(ctx, e.DevID)
	if err != nil {
		return IssuedChallenge{}, err
	}
	if err := d.VerifyAccess(c.p.AllowNoneProfile); err != nil {
		return IssuedChallenge{}, err
	}
	var ns [16]byte
	if _, err := io.ReadFull(c.rand, ns[:]); err != nil {
		return IssuedChallenge{}, err
	}
	s1 := dhs.TruncateMicro(c.now())
	expires := s1.Add(c.p.ChallengeValidity)
	ch, err := c.repos.Challenges.Issue(ctx, dhs.Issuance{DongleID: d.ID, S1: s1, ExpiresAt: expires, Nonce: ns[:],
		MaxPending: c.p.MaxPending})
	if err != nil {
		if errors.Is(err, dhs.ErrPendingChallenges) {
			return IssuedChallenge{RetryAfter: c.oldestPending()}, err
		}
		return IssuedChallenge{}, err
	}
	msg := protocol.Challenge{CS: uint32(ch.CS), NS: ns, AckSeq: uint32(ch.ContiguousSeq), CConf: uint32(ch.CConf)}
	macs, err := c.kms.HMACAuth(port.WithKMSPhase(ctx, port.PhasePre), d.AuthKey,
		[][]byte{protocol.TagInput(protocol.ChallengeType, d.DevID, msg.Payload())})
	if err != nil {
		return IssuedChallenge{}, err
	}
	tlv, err := msg.Build(tag16(macs[0]))
	if err != nil {
		return IssuedChallenge{}, err
	}
	c.log.Info("handshake.desafio_emitido", "dongle", d.DevID.Hex(), "c_s", msg.CS, "ack_seq", msg.AckSeq)
	c.met.Handshake("challenge_issued")
	return IssuedChallenge{TLV: tlv, ExpiresAt: expires}, nil
}

// oldestPending: sem consulta dedicada, usa um terço da validade do desafio como teto conservador
// (mínimo 1 s) para o Retry-After.
func (c *IssueChallenge) oldestPending() time.Duration {
	return time.Second * time.Duration(max(1, int(c.p.ChallengeValidity.Seconds())/3))
}
