package port

import (
	"context"

	"tccelta/backend/internal/domain/protocol"
)

// Inspector é a leitura somente-consulta do que o servidor gravou, para o simulador conferir os cenários.
// Não pertence a nenhum agregado: atravessa dongle, lacunas, boots, leituras e alertas.
type Inspector interface {
	// DongleState devolve o dongle e seu dongle_state pelo DevId.
	DongleState(ctx context.Context, dev protocol.DevID) (InspectedState, error)
	Gaps(ctx context.Context, dongleID int32) ([]InspectedGap, error)
	Boots(ctx context.Context, dongleID int32) ([]InspectedBoot, error)
	CountReadings(ctx context.Context, dongleID int32) (int64, error)
	CountPositions(ctx context.Context, dongleID int32) (int64, error)
	CountRejected(ctx context.Context, dongleID int32) (int64, error)
	CountAnchors(ctx context.Context, dongleID int32) (int64, error)
	CountAlerts(ctx context.Context, dongleID int32, alertType string) (int64, error)
	// ReadingTime é a hora reconstruída (ms desde a época) da leitura mais recente de um boot com tempo.
	ReadingTime(ctx context.Context, dongleID int32) (float64, error)
}

// InspectedState é o recorte de dongle_state conferido pelo simulador.
type InspectedState struct {
	DongleID                   int32
	ContiguousSeq, HeadSeq     int64
	CConf                      int64
	BootGeneration, LastBootID int32
}

// InspectedGap é uma lacuna registrada.
type InspectedGap struct {
	FromSeq, ToSeq int64
	Cause          string
}

// InspectedBoot é um boot com sua estimativa de tempo (nil enquanto não há).
type InspectedBoot struct {
	BootGeneration, BootID int32
	TimeSource             *string
	AMs, B                 *float64
	BaseUncertaintyMs      *int32
	TimePending            bool
}
