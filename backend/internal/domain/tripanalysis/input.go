package tripanalysis

import (
	"fmt"
	"time"
)

// Tipos de entrada da viagem.
const (
	EntryBoot     = "boot"
	EntryInterval = "interval"
)

// Entry é a definição da viagem: exatamente um boot, ou exatamente um intervalo de tempo.
type Entry struct {
	Type           string
	DongleID       int32
	BootGeneration *int32
	BootID         *int32
	T0, T1         *time.Time
}

// Validate rejeita duas entradas, nenhuma, tipo desconhecido e t1 <= t0.
func (e Entry) Validate() error {
	switch e.Type {
	case EntryBoot:
		if e.BootID == nil {
			return fmt.Errorf("%w: boot sem boot_id", ErrInvalidEntry)
		}
		if e.T0 != nil || e.T1 != nil {
			return fmt.Errorf("%w: boot e intervalo juntos", ErrInvalidEntry)
		}
	case EntryInterval:
		if e.T0 == nil || e.T1 == nil {
			return fmt.Errorf("%w: intervalo sem t0 e t1", ErrInvalidEntry)
		}
		if e.BootID != nil || e.BootGeneration != nil {
			return fmt.Errorf("%w: boot e intervalo juntos", ErrInvalidEntry)
		}
		if !e.T1.After(*e.T0) {
			return fmt.Errorf("%w: t1 deve ser maior que t0", ErrInvalidEntry)
		}
	default:
		return fmt.Errorf("%w: tipo %q", ErrInvalidEntry, e.Type)
	}
	return nil
}

// Sample é uma leitura: t_ms no eixo da análise e o valor decodificado.
type Sample struct {
	TMs   int64
	Value float64
}

// PIDSeries são as leituras de um PID, em ordem de t_ms.
type PIDSeries struct {
	PID     uint8
	Samples []Sample
}

// Position é um ponto do GPS no eixo da análise.
type Position struct {
	TMs      int64
	Lat, Lon float64
}

// Clock é a reta de tempo do boot (hora(t) = AMs + (1+B)·t) e a incerteza da parte.
type Clock struct {
	AMs, B        float64
	UncertaintyMs int32
}

// PartInput reúne os dados contínuos de um boot dentro da viagem, já no eixo da análise.
type PartInput struct {
	BootGeneration, BootID int32
	// Readings tem uma série por PID, em ordem crescente de PID.
	Readings  []PIDSeries
	Positions []Position
	// Clock é nulo se o boot não tem conversão para hora absoluta.
	Clock *Clock
}

// Warning é um aviso do contrato: codigo e, quando aplicável, detalhe.
type Warning struct {
	Code   string         `json:"codigo"`
	Detail map[string]any `json:"detalhe,omitempty"`
}

// Códigos de aviso.
const (
	WarnNoGPS               = "sem_gps"
	WarnNoAbsoluteTime      = "sem_hora_absoluta"
	WarnBootNoTime          = "boot_sem_hora"
	WarnOverlappingParts    = "partes_sobrepostas"
	WarnNoReadings          = "sem_leituras"
	WarnEngineNeverOn       = "motor_nunca_ligado"
	WarnLowMAFCoverage      = "cobertura_maf_baixa"
	WarnFuelMayBeUnderrated = "consumo_pode_estar_subestimado"
	WarnIdleFallback        = "marcha_lenta_reserva"
	WarnPartialData         = "dados_parciais"
)

// Input é tudo que Analyze precisa.
type Input struct {
	Entry  Entry
	Parts  []PartInput
	Params Params
	// EntryWarnings são avisos que só o carregador sabe (boot_sem_hora).
	EntryWarnings []Warning
	// Partial marca a viagem calculada com o limite final ou com blocos ainda faltando (dados_parciais).
	Partial bool
}
