package handshake

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"tccelta/backend/internal/domain/dongle"
	dhs "tccelta/backend/internal/domain/handshake"
	"tccelta/backend/internal/domain/occurrence"
	"tccelta/backend/internal/domain/protocol"
	"tccelta/backend/internal/domain/timeline"
	"tccelta/backend/internal/observability"
	"tccelta/backend/internal/repository"
	"tccelta/backend/internal/usecase/port"
)

// RespondChallengeInput é a resposta do dongle. S2 é o instante de chegada da requisição.
type RespondChallengeInput struct {
	DevID protocol.DevID
	S2    time.Time
	TLV   []byte
}

// HandshakeResult é a saída de RespondChallenge.
type HandshakeResult struct {
	AckTLV         []byte
	Token          string
	TokenExpiresAt time.Time
	SuggestedTLat  time.Duration
	WithAnchor     bool
	AckSeq         uint32
	ValidityS      int
}

// RespondChallenge valida a resposta, ancora o tempo e emite o token
// (POST /v1/dongles/{id}/desafio/resposta).
type RespondChallenge struct {
	repos repository.Repositories
	tx    repository.Transactor
	kms   port.Authenticator
	token port.TokenIssuer
	met   port.MetricsHandshake
	now   port.Clock
	log   *slog.Logger
	p     Params
}

// NewRespondChallenge liga o caso de uso às suas portas.
func NewRespondChallenge(repos repository.Repositories, tx repository.Transactor, kms port.Authenticator,
	token port.TokenIssuer, met port.MetricsHandshake, now port.Clock, log *slog.Logger, p Params) *RespondChallenge {
	return &RespondChallenge{repos: repos, tx: tx, kms: kms, token: token, met: met, now: now, log: log, p: p}
}

var errRollback = errors.New("rollback")

func stateOf(e dongle.State) dhs.State {
	return dhs.State{ContiguousSeq: uint32(e.ContiguousSeq), HeadSeq: uint32(e.HeadSeq), CConf: uint32(e.CConf),
		BootGeneration: uint16(e.BootGeneration), LastBootID: uint16(e.LastBootID)}
}

// Run processa o TLV 0x02 já extraído do JSON.
func (c *RespondChallenge) Run(ctx context.Context, e RespondChallengeInput) (HandshakeResult, error) {
	d, err := c.repos.Dongles.FindByDevID(ctx, e.DevID)
	if err != nil {
		return HandshakeResult{}, err
	}
	if err := d.VerifyAccess(c.p.AllowNoneProfile); err != nil {
		return HandshakeResult{}, err
	}
	s2 := dhs.MicroCeiling(e.S2)
	t, n, err := protocol.ReadTLV(e.TLV)
	if err != nil || n != len(e.TLV) || t.T != protocol.ResponseType {
		return HandshakeResult{}, protocol.ErrUnexpectedType
	}
	r, payload, tag, err := protocol.DecodeResponse(t.V)
	if err != nil {
		return HandshakeResult{}, err
	}

	// 4. pré-checagem sem lock
	ds, err := c.repos.Challenges.Find(ctx, d.ID, int64(r.CS))
	if err != nil {
		if errors.Is(err, dhs.ErrInvalidChallenge) {
			c.met.Handshake("invalid_challenge")
		}
		return HandshakeResult{}, err
	}
	if !protocol.CompareTag(ds.Nonce, r.NS[:]) || ds.AnsweredAt != nil || !ds.ExpiresAt.After(s2) {
		c.met.Handshake("invalid_challenge")
		return HandshakeResult{}, dhs.ErrInvalidChallenge
	}
	est, err := c.repos.States.Read(ctx, d.ID)
	if err != nil {
		return HandshakeResult{}, err
	}
	if int64(r.CS) <= est.CConf {
		c.met.Handshake("stale_c_s")
		return HandshakeResult{}, dhs.ErrStaleCS
	}

	// 5. tag K_auth
	macs, err := c.kms.HMACAuth(observability.WithKMSPhase(ctx, observability.PhasePre), d.AuthKey,
		[][]byte{protocol.TagInput(protocol.ResponseType, d.DevID, payload)})
	if err != nil {
		return HandshakeResult{}, err
	}
	if !protocol.CompareTag(tag16(macs[0]), tag) {
		_ = c.repos.Challenges.MarkInvalidTag(ctx, d.ID, int64(r.CS), r.NS[:])
		c.log.Warn("handshake.tag_invalida", "dongle", d.DevID.Hex(), "c_s", r.CS)
		c.met.Handshake("invalid_tag")
		return HandshakeResult{}, dhs.ErrInvalidTag
	}

	// 6. JWT (fora da transação)
	emit, err := c.token.Issue(ctx, d.DevID, r.CS)
	if err != nil {
		return HandshakeResult{}, err
	}

	// 7. transação
	var res HandshakeResult
	var errAfterCommit error
	err = c.tx.InTransaction(ctx, func(ctx context.Context, rp repository.Repositories) error {
		errAfterCommit = nil
		if err := rp.Dongles.Lock(ctx, d.ID); err != nil {
			return err
		}
		bootID := int32(r.BootID)
		head, tail := int64(r.HeadSeq), int64(r.TailSeq)
		cs := int64(r.CS)
		s1, err := rp.Challenges.Consume(ctx, dhs.Consumption{DongleID: d.ID, CS: cs, Nonce: r.NS[:], S2: s2, BootID: bootID,
			TMs: int64(r.TMs), HeadSeq: head, TailSeq: tail})
		if err != nil {
			if errors.Is(err, dhs.ErrInvalidChallenge) {
				errAfterCommit = dhs.ErrInvalidChallenge
				return errRollback
			}
			return err
		}
		er, err := rp.States.Read(ctx, d.ID)
		if err != nil {
			return err
		}
		est := stateOf(er)
		if cs <= er.CConf {
			errAfterCommit = dhs.ErrStaleCS
			return errRollback
		}
		ack, loss, err := dhs.ApplyTail(est, r.TailSeq, r.HeadSeq)
		if err != nil {
			if ferr := rp.Challenges.Finalize(ctx, dhs.Finalization{DongleID: d.ID, CS: cs, Result: dhs.ResultInconsistent}); ferr != nil {
				return ferr
			}
			c.log.Warn("handshake.inconsistente", "dongle", d.DevID.Hex(), "c_s", r.CS, "head", r.HeadSeq, "tail", r.TailSeq, "head_salvo", est.HeadSeq)
			errAfterCommit = dhs.ErrInconsistentResponse
			return nil
		}
		var lossStart, lossEnd *int64
		if loss != nil {
			start, end := int64(loss.Start), int64(loss.End)
			if err := rp.Occurrences.InsertGap(ctx, occurrence.Gap{DongleID: d.ID, From: start, To: end,
				Cause: occurrence.CauseOverflow, RegisteredAt: s2, CS: &cs}); err != nil {
				return err
			}
			c.met.Gap(occurrence.CauseOverflow)
			lossStart, lossEnd = &start, &end
		}
		rtt := s2.Sub(s1)
		withAnchor, clock := dhs.EvaluateAnchor(s1, s2, c.p.AnchorRTTMax)
		if rtt >= 0 {
			c.met.AnchorRTT(rtt)
		}
		gen := dhs.HandshakeGeneration(est, r.BootID)
		genI := int32(gen)
		if clock {
			_ = rp.Occurrences.InsertAlert(ctx, occurrence.Alert{DongleID: d.ID, BootGeneration: &genI, BootID: &bootID,
				Type: occurrence.AlertServerClockRegressed, Detail: []byte(`{"rtt_ms":` + strconv.FormatInt(rtt.Milliseconds(), 10) + `}`)})
		}
		refs, err := rp.Boots.LastTMs(ctx, d.ID, []int32{bootID}, genI, genI)
		if err != nil {
			return err
		}
		ref := int64(-1)
		for _, x := range refs {
			if x.Generation == genI && x.BootID == bootID {
				ref = x.LastTMs
			}
		}
		tMs := timeline.Unwrap(r.TMs, ref)
		if err := rp.Boots.EnsurePending(ctx, d.ID, genI, bootID, c.now()); err != nil {
			return err
		}
		if withAnchor {
			if err := rp.Boots.InsertAnchor(ctx, dhs.Anchor{DongleID: d.ID, BootGeneration: genI, BootID: bootID,
				TMs: tMs, Nonce: r.NS[:], S1: s1, S2: s2}); err != nil {
				return err
			}
		}
		am := protocol.Ack{AckSeq: ack, CConf: r.CS}
		if !withAnchor {
			am.Flags = protocol.FlagAckAnchorFailed
		}
		macs, err := c.kms.HMACAuth(observability.WithKMSPhase(ctx, observability.PhaseTx), d.AuthKey,
			[][]byte{protocol.TagInput(protocol.AckType, d.DevID, am.Payload())})
		if err != nil {
			return err
		}
		ackTLV := am.Build(tag16(macs[0]))
		ackI := int64(ack)
		if err := rp.States.UpdateHandshake(ctx, dhs.Update{DongleID: d.ID, ContiguousSeq: ackI, HeadSeq: head,
			TailSeq: tail, S2: s2, Now: c.now(), CConf: cs, LastAck: ackTLV}); err != nil {
			return err
		}
		result := dhs.ResultAccepted
		if !withAnchor {
			result = dhs.ResultAcceptedNoAnchor
		}
		jti := emit.JTI
		if err := rp.Challenges.Finalize(ctx, dhs.Finalization{DongleID: d.ID, CS: cs, Result: result, AckSeq: &ackI,
			LossStart: lossStart, LossEnd: lossEnd, TokenJTI: &jti, UnwrappedTMs: &tMs, BootGeneration: &genI}); err != nil {
			return err
		}
		if err := rp.Boots.NotifyPending(ctx, fmt.Sprintf("%d:%d:%d", d.ID, gen, r.BootID)); err != nil {
			return err
		}
		res = HandshakeResult{AckTLV: ackTLV, Token: emit.Token, TokenExpiresAt: emit.ExpiresAt, WithAnchor: withAnchor, AckSeq: ack,
			SuggestedTLat: c.p.SuggestedTLat, ValidityS: int(c.token.Validity().Seconds())}
		c.log.Info("handshake.aceito", "dongle", d.DevID.Hex(), "c_s", r.CS, "geracao_boot", gen, "boot_id", r.BootID,
			"rtt_ms", rtt.Milliseconds(), "com_ancora", withAnchor, "ack_seq", ack)
		return nil
	})
	if errors.Is(err, errRollback) {
		err = errAfterCommit
	}
	if err != nil {
		return HandshakeResult{}, err
	}
	if errAfterCommit != nil {
		c.met.Handshake("inconsistent")
		return HandshakeResult{}, errAfterCommit
	}
	if res.WithAnchor {
		c.met.Handshake("accepted")
	} else {
		c.met.Handshake("accepted_no_anchor")
	}
	return res, nil
}
