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
	// Register insere o dongle e o dongle_state zerado. Cabe ao chamador abrir a transação.
	Register(ctx context.Context, r dongle.Registration) (dongleID int32, err error)
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
	// Expire marca como expirados os desafios sem resposta vencidos antes de now e devolve quantos foram.
	Expire(ctx context.Context, now time.Time) (int64, error)
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
	// TakePending retira da fila o boot pendente mais antigo, travando-o até o fim da transação. found=false se a fila está vazia.
	TakePending(ctx context.Context) (b timeline.PendingBoot, found bool, err error)
	// ListAnchors devolve as âncoras do boot por t_ms, com S1 e S2 em milissegundos desde a época.
	ListAnchors(ctx context.Context, k timeline.BootKey) ([]timeline.Anchor, error)
	// PreviousAnchored e NextAnchored devolvem o boot ancorado vizinho (por first_seq) ou nil se não há (ou se falta dado).
	PreviousAnchored(ctx context.Context, dongleID int32, firstSeq int64) (*timeline.NeighborReference, error)
	NextAnchored(ctx context.Context, dongleID int32, firstSeq int64) (*timeline.NeighborReference, error)
	// UpdateTime grava a estimativa e tira o boot da fila; est nil limpa a estimativa (sem origem de tempo).
	UpdateTime(ctx context.Context, k timeline.BootKey, est *timeline.Estimate, now time.Time) error
	// MarkAnchorlessNeighbors recoloca na fila os boots sem âncora entre os boots ancorados vizinhos de firstSeq.
	MarkAnchorlessNeighbors(ctx context.Context, dongleID int32, firstSeq int64, now time.Time) (int64, error)
	// MarkGold marca para o gold os boots que cobrem a faixa de seq.
	MarkGold(ctx context.Context, dongleID int32, fromSeq, toSeq int64) error
	CountPending(ctx context.Context) (int64, error)
}

// Blocks é bronze.raw_block, os rejeitados e as linhas derivadas (silver).
type Blocks interface {
	// InsertRawBlocks devolve quantos blocos foram gravados (os repetidos não contam).
	InsertRawBlocks(ctx context.Context, dongleID int32, epoch int64, receivedAt time.Time, blocks []ingestion.RawBlock) (int, error)
	RegisterRejected(ctx context.Context, r ingestion.Rejected) error
	CopyReadings(ctx context.Context, dongleID int32, receivedAt time.Time, ls []pid.Reading) error
	CopyPositions(ctx context.Context, dongleID int32, receivedAt time.Time, ps []pid.Position) error
	// CopyReadingsAt e CopyPositionsAt gravam linhas cada uma com o recebimento do seu bloco (reprocessamento).
	CopyReadingsAt(ctx context.Context, dongleID int32, ls []ReadingAt) error
	CopyPositionsAt(ctx context.Context, dongleID int32, ps []PositionAt) error
	// ListData lê os blocos de dados (sem marcador) da faixa de seq, em ordem, até limit.
	ListData(ctx context.Context, dongleID int32, fromSeq, toSeq int64, limit int32) ([]ingestion.StoredBlock, error)
	CountReadings(ctx context.Context, dongleID int32, fromSeq, toSeq int64) (int64, error)
	// DeleteReadings e DeletePositions removem as linhas derivadas dos blocos da faixa recebidos entre recvMin e recvMax.
	DeleteReadings(ctx context.Context, dongleID int32, fromSeq, toSeq int64, recvMin, recvMax time.Time) (int64, error)
	DeletePositions(ctx context.Context, dongleID int32, fromSeq, toSeq int64, recvMin, recvMax time.Time) (int64, error)
}

// Occurrences são lacunas e alertas.
type Occurrences interface {
	InsertGap(ctx context.Context, l occurrence.Gap) error
	InsertAlert(ctx context.Context, a occurrence.Alert) error
	// InsertAlertIfNew não repete o alerta do mesmo boot e tipo dentro de 24 h antes de now (BootGeneration e BootID obrigatórios).
	InsertAlertIfNew(ctx context.Context, a occurrence.Alert, now time.Time) error
}

// AnchorGroups são os grupos de âncoras de cada boot (bronze para a reta de tempo).
type AnchorGroups interface {
	Delete(ctx context.Context, k timeline.BootKey) error
	// Copy grava os grupos na ordem, numerados de 1.
	Copy(ctx context.Context, k timeline.BootKey, computedAt time.Time, groups []timeline.Group) error
}

// Pids é o catálogo de PIDs.
type Pids interface {
	List(ctx context.Context) ([]pid.Def, error)
}

// ReadingAt e PositionAt são linhas derivadas com o recebimento do bloco de origem.
type (
	ReadingAt struct {
		pid.Reading
		ReceivedAt time.Time
	}
	PositionAt struct {
		pid.Position
		ReceivedAt time.Time
	}
)

// Repositories agrupa os repositórios; numa transação, todos enxergam a mesma tx.
type Repositories struct {
	Dongles      Dongles
	Challenges   Challenges
	States       States
	Boots        Boots
	Blocks       Blocks
	Occurrences  Occurrences
	AnchorGroups AnchorGroups
	Pids         Pids
}

// Transactor executa fn numa transação READ COMMITTED com timeouts locais. Deadlock e falha de
// serialização repetem fn uma vez; o que sobrar vira ErrDatabaseBusy. Erro de fn desfaz a transação.
type Transactor interface {
	InTransaction(ctx context.Context, fn func(ctx context.Context, r Repositories) error) error
}

// Notifications entrega os sinais assíncronos do banco (LISTEN/NOTIFY), fora de qualquer transação.
type Notifications interface {
	// Listen entrega as cargas do canal até ctx terminar. Ao reconectar emite uma carga vazia
	// (notificações podem ter sido perdidas) e chama reconnections, se não for nil.
	Listen(ctx context.Context, channel string, reconnections func()) <-chan string
}
