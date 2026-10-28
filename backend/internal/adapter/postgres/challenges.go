package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"tccelta/backend/internal/database/dbgen"
	"tccelta/backend/internal/domain/handshake"
)

type challenges struct{ q dbgen.Querier }

func (r *challenges) Issue(ctx context.Context, e handshake.Issuance) (handshake.Challenge, error) {
	row, err := r.q.IssueChallenge(ctx, dbgen.IssueChallengeParams{
		NowS: e.S1.Unix(), DongleID: e.DongleID, S1: e.S1, MaxPending: int32(e.MaxPending),
		Nonce: e.Nonce, ExpiresAt: e.ExpiresAt, FlagsC: 0,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return handshake.Challenge{}, handshake.ErrPendingChallenges
		}
		return handshake.Challenge{}, err
	}
	return handshake.Challenge{CS: row.CS, ContiguousSeq: row.ContiguousSeq, CConf: row.CConf}, nil
}

func (r *challenges) Find(ctx context.Context, dongleID int32, cs int64) (handshake.StoredChallenge, error) {
	row, err := r.q.FindChallenge(ctx, dongleID, cs)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return handshake.StoredChallenge{}, handshake.ErrInvalidChallenge
		}
		return handshake.StoredChallenge{}, err
	}
	return handshake.StoredChallenge{Nonce: row.Nonce, S1: row.S1, ExpiresAt: row.ExpiresAt, AnsweredAt: row.AnsweredAt}, nil
}

func (r *challenges) Consume(ctx context.Context, c handshake.Consumption) (time.Time, error) {
	s2, boot, tms, head, tail := c.S2, c.BootID, c.TMs, c.HeadSeq, c.TailSeq
	s1, err := r.q.ConsumeChallenge(ctx, dbgen.ConsumeChallengeParams{S2: &s2, BootID: &boot, TMs: &tms,
		HeadSeq: &head, TailSeq: &tail, DongleID: c.DongleID, CS: c.CS, Nonce: c.Nonce})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return time.Time{}, handshake.ErrInvalidChallenge
		}
		return time.Time{}, err
	}
	return s1, nil
}

func (r *challenges) Finalize(ctx context.Context, f handshake.Finalization) error {
	res := f.Result
	return r.q.FinalizeChallenge(ctx, dbgen.FinalizeChallengeParams{Result: &res, AckSeq: f.AckSeq,
		LossStart: f.LossStart, LossEnd: f.LossEnd, TokenJti: f.TokenJTI, UnwrappedTMs: f.UnwrappedTMs,
		BootGeneration: f.BootGeneration, DongleID: f.DongleID, CS: f.CS})
}

func (r *challenges) MarkInvalidTag(ctx context.Context, dongleID int32, cs int64, nonce []byte) error {
	return r.q.MarkInvalidTag(ctx, dongleID, cs, nonce)
}

func (r *challenges) Expire(ctx context.Context, now time.Time) (int64, error) {
	return r.q.ExpireChallenges(ctx, now)
}
