package port

import (
	"context"
	"time"

	"tccelta/backend/internal/domain/dongle"
	"tccelta/backend/internal/domain/handshake"
	"tccelta/backend/internal/domain/ingestion"
	"tccelta/backend/internal/domain/occurrence"
	"tccelta/backend/internal/domain/pid"
	"tccelta/backend/internal/domain/protocol"
	"tccelta/backend/internal/domain/timeline"
)

// Dongles é o cadastro. FindByDevID devolve dongle.ErrUnknown quando não há cadastro.
type Dongles interface {
	FindByDevID(ctx context.Context, dev protocol.DevID) (dongle.Dongle, error)
	// Lock toma o lock de aconselhamento do dongle até o fim da transação.
	Lock(ctx context.Context, dongleID int32) error
}

// Challenges guarda o ciclo de vida dos desafios do handshake.
type Challenges interface {
	// Issue devolve handshake.ErrPendingChallenges quando o dongle já tem pendentes demais.
	Issue(ctx context.Context, e handshake.Issuance) (handshake.Challenge, error)
	// Find devolve handshake.ErrInvalidChallenge quando o desafio não existe.
	Find(ctx context.Context, dongleID int32, cs int64) (handshake.StoredChallenge, error)
	// Consume devolve S1 do desafio; handshake.ErrInvalidChallenge se não estiver pendente e no prazo.
	Consume(ctx context.Context, c handshake.Consumption) (s1 time.Time, err error)
	Finalize(ctx context.Context, f handshake.Finalization) error
	MarkInvalidTag(ctx context.Context, dongleID int32, cs int64, nonce []byte) error
}

// States é dongle_state.
type States interface {
	Read(ctx context.Context, dongleID int32) (dongle.State, error)
	// ReadSnapshot lê ContiguousSeq, CConf e LastAck.
	ReadSnapshot(ctx context.Context, dongleID int32) (dongle.State, error)
	ReadLastAck(ctx context.Context, dongleID int32) ([]byte, error)
	UpdateHandshake(ctx context.Context, a handshake.Update) error
	UpdateIngestion(ctx context.Context, a ingestion.StateUpdate) error
}

// Boots é a tabela de boots (tempo por boot_id e âncoras).
type Boots interface {
	// LastTMs devolve as referências candidatas nas gerações gMin e gMax para os boot_ids dados.
	LastTMs(ctx context.Context, dongleID int32, bootIDs []int32, gMin, gMax int32) ([]timeline.RefBoot, error)
	EnsurePending(ctx context.Context, dongleID, generation, bootID int32, now time.Time) error
	UpdateIngestion(ctx context.Context, dongleID int32, now time.Time, aggregates []ingestion.BootAggregate) error
	InsertAnchor(ctx context.Context, a handshake.Anchor) error
	NotifyPending(ctx context.Context, key string) error
}

// Blocks é bronze.raw_block, os rejeitados e as linhas derivadas (silver).
type Blocks interface {
	// InsertRawBlocks devolve quantos blocos foram gravados (os repetidos não contam).
	InsertRawBlocks(ctx context.Context, dongleID int32, epoch int64, receivedAt time.Time, blocks []ingestion.RawBlock) (int, error)
	RegisterRejected(ctx context.Context, r ingestion.Rejected) error
	CopyReadings(ctx context.Context, dongleID int32, receivedAt time.Time, ls []pid.Reading) error
	CopyPositions(ctx context.Context, dongleID int32, receivedAt time.Time, ps []pid.Position) error
}

// Occurrences são lacunas e alertas.
type Occurrences interface {
	InsertGap(ctx context.Context, l occurrence.Gap) error
	InsertAlert(ctx context.Context, a occurrence.Alert) error
}

// Pids é o catálogo de PIDs.
type Pids interface {
	List(ctx context.Context) ([]pid.Def, error)
}

// Repositories agrupa os repositórios; numa transação, todos enxergam a mesma tx.
type Repositories struct {
	Dongles     Dongles
	Challenges  Challenges
	States      States
	Boots       Boots
	Blocks      Blocks
	Occurrences Occurrences
	Pids        Pids
}

// Transactor executa fn numa transação READ COMMITTED com timeouts locais. Deadlock e falha de
// serialização repetem fn uma vez; o que sobrar vira ErrDatabaseBusy. Erro de fn desfaz a transação.
type Transactor interface {
	InTransaction(ctx context.Context, fn func(ctx context.Context, r Repositories) error) error
}
