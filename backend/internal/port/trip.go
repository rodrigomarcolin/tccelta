package port

import (
	"context"
	"time"

	"github.com/google/uuid"

	"tccelta/backend/internal/domain/trip"
)

// Trips é gold.trip e as tabelas filhas da análise.
type Trips interface {
	// ListWaiting devolve, sem lock, as viagens em pending ou waiting_data, da mais antiga para a mais nova.
	ListWaiting(ctx context.Context) ([]trip.Trip, error)
	// MarkWaiting passa a viagem de pending para waiting_data.
	MarkWaiting(ctx context.Context, id uuid.UUID) error
	// ResolveGeneration grava em boot_generation a maior geração que tem o boot_id, se ela existir e a coluna for nula.
	ResolveGeneration(ctx context.Context, id uuid.UUID, dongleID, bootID int32) (generation int32, found bool, err error)
	// Take trava a viagem (pending ou waiting_data) até o fim da transação. found=false se outra transação a tem ou ela mudou de estado.
	Take(ctx context.Context, id uuid.UUID) (t trip.Trip, found bool, err error)
	// TakeRecalc trava a viagem pronta com recalc_pending mais antiga.
	TakeRecalc(ctx context.Context) (t trip.Trip, found bool, err error)
	// Save apaga e regrava as linhas filhas e atualiza a viagem: ready, versão, horário, faixa de seq, provisional, recalc_pending=false.
	Save(ctx context.Context, id uuid.UUID, c trip.Calculation) error
	// Fail marca failed uma viagem nova (e libera o nome). Uma viagem pronta mantém o resultado e só perde o recalc_pending.
	Fail(ctx context.Context, id uuid.UUID) error
	// ListProvisional devolve as viagens prontas e provisórias que ainda não estão marcadas para recálculo.
	ListProvisional(ctx context.Context) ([]trip.Provisional, error)
	MarkRecalc(ctx context.Context, ids []uuid.UUID) error
	ClearProvisional(ctx context.Context, id uuid.UUID) error
	// MarkRecalcByRange marca recalc_pending nas viagens prontas do dongle cuja faixa [first_seq, last_seq] cruza [fromSeq, toSeq].
	MarkRecalcByRange(ctx context.Context, dongleID int32, fromSeq, toSeq int64) error
	// Notify acorda o worker (trip_pending). Em transação, o sinal só sai no commit.
	Notify(ctx context.Context) error
	CountQueue(ctx context.Context) (trip.Queue, error)

	// Insert grava a viagem nova em pending (a API2). trip.ErrDriverRepeated se já há viagem do motorista na categoria e
	// trip.ErrTripRepeated se a mesma entrada já foi processada (índice único; só numa corrida, pois o chamador confere antes com FindSame).
	Insert(ctx context.Context, d trip.Draft) (trip.Trip, error)
	// FindSame devolve a viagem que impede criar d: a mesma entrada (boot com geração, ou intervalo exato) no dongle, de qualquer
	// motorista, que não falhou nem foi excluída. Prefere a pronta, depois a mais antiga. A geração de d já deve estar resolvida.
	FindSame(ctx context.Context, d trip.Draft) (t trip.Trip, found bool, err error)
	// NotifyTrip acorda o worker com o id da viagem criada (trip_pending). Em transação, o sinal só sai no commit.
	NotifyTrip(ctx context.Context, id uuid.UUID) error
	// Find devolve trip.ErrNotFound se a viagem não existe ou foi excluída.
	Find(ctx context.Context, id uuid.UUID) (trip.Trip, error)
	// List devolve uma página da listagem (da mais nova para a mais antiga, sem as excluídas), com o total que casa com a busca.
	// Roda numa transação própria REPEATABLE READ somente leitura, para total, linhas e rotas serem consistentes.
	List(ctx context.Context, q trip.ListQuery) (trip.Page, error)
	// Exclude marca a viagem como excluída (exclusão lógica: excluded_at). trip.ErrNotFound se ela não existe ou já foi excluída.
	// Espera a trava de uma viagem em cálculo; com o lock_timeout da transação vira ErrDatabaseBusy.
	Exclude(ctx context.Context, id uuid.UUID) error
	// Analysis lê a viagem e suas tabelas filhas numa transação própria REPEATABLE READ somente leitura, sem misturar linhas de
	// antes e depois de um recálculo. Com a viagem fora de ready devolve só Analysis.Trip. trip.ErrNotFound se não existe.
	Analysis(ctx context.Context, id uuid.UUID) (trip.Analysis, error)
}

// TripSources é a leitura da silver (boot, bronze.raw_block, reading, position) para o cálculo da viagem.
type TripSources interface {
	// FindBoot devolve found=false se o boot não existe.
	FindBoot(ctx context.Context, dongleID, generation, bootID int32) (b trip.Boot, found bool, err error)
	// IntervalBoots devolve os boots com conversão de hora cuja faixa (hora absoluta, ms desde a época) toca [fromMs, toMs).
	IntervalBoots(ctx context.Context, dongleID int32, fromMs, toMs float64) ([]trip.Boot, error)
	// UnconvertedBoots devolve os boots sem conversão de hora que podem cair no intervalo [t0Ms, t1Ms) (ver spec API1).
	UnconvertedBoots(ctx context.Context, dongleID int32, t0Ms, t1Ms float64) ([]trip.Boot, error)
	// ReceivedRange é o menor e o maior received_at dos blocos de dados do boot; found=false se não há blocos.
	ReceivedRange(ctx context.Context, dongleID int32, b trip.Boot) (from, to time.Time, found bool, err error)
	LoadReadings(ctx context.Context, q trip.ReadingQuery) ([]trip.ReadingRow, error)
	LoadPositions(ctx context.Context, q trip.ReadingQuery) ([]trip.PositionRow, error)
	// NextBootStart devolve o first_seq do primeiro boot do dongle que começa depois de afterSeq.
	NextBootStart(ctx context.Context, dongleID int32, afterSeq int64) (seq int64, found bool, err error)
	// MaxBlockTime devolve a hora absoluta (ms) do t0 do bloco mais recente já convertido com seq em [fromSeq, toSeq].
	MaxBlockTime(ctx context.Context, dongleID int32, fromSeq, toSeq int64) (ms float64, found bool, err error)
}

// TripSeeder cria viagens de teste (simulador e e2e); a API de viagens é a API2. Roda com o papel dono das tabelas.
type TripSeeder interface {
	SeedBootTrip(ctx context.Context, s SeedTrip) (uuid.UUID, error)
	SeedIntervalTrip(ctx context.Context, s SeedTrip) (uuid.UUID, error)
}

// SeedTrip descreve uma viagem a criar. Para a entrada boot, BootGeneration e BootID; para intervalo, Start e End.
type SeedTrip struct {
	Category, Driver, Fuel string
	DongleID               int32
	BootGeneration, BootID int32
	Start, End             time.Time
	TargetSeq              int64
}

// TripInspection é o estado de uma viagem, para o simulador conferir.
type TripInspection struct {
	Status                string
	Provisional           bool
	CalculatedThroughSeq  *int64
	Parts                 int64
	DistanceKm, EngineOnS float64
}
