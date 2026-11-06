// Package tripanalysis calcula a análise de uma viagem (spec .spec/analise-de-viagem/02) a partir de leituras e posições já
// carregadas no eixo de tempo da análise. É puro: sem banco, sem relógio e sem dependência das portas. A conversão de hora e a
// seleção dos dados são do chamador (usecase/tripcalc).
package tripanalysis

import (
	"errors"
	"fmt"
)

// CalculationVersion é a versao_calculo gravada com a análise.
const CalculationVersion = "1"

// Erros de validação.
var (
	ErrInvalidParams = errors.New("parametros_invalidos")
	ErrInvalidEntry  = errors.New("entrada_invalida")
)

// FuelPreset é um combustível da spec 02.
type FuelPreset struct {
	ID        string
	Name      string
	AFR       float64
	DensityGL float64
}

// FuelPresets são os presets da spec 02, na ordem da tabela.
var FuelPresets = []FuelPreset{
	{"gasolina_e30", "Gasolina E30", 13.0, 750},
	{"gasolina_e32", "Gasolina E32", 13.0, 750},
	{"gasolina_e27", "Gasolina E27", 13.2, 754},
	{"gasolina_pura", "Gasolina pura", 14.7, 745},
	{"etanol_hidratado", "Etanol hidratado", 9.0, 809},
	{"diesel", "Diesel", 14.5, 832},
}

// FuelCustom é o combustível que exige afr e densidade informados (fora da API1).
const FuelCustom = "personalizado"

// Params são os parâmetros do cálculo (todos os da spec 02, menos o trecho). Os json são os nomes do contrato.
type Params struct {
	Fuel      string  `json:"combustivel"`
	AFR       float64 `json:"afr"`
	DensityGL float64 `json:"densidade_g_l"`
	// IdleRPM nulo = estimar a marcha lenta.
	IdleRPM             *float64 `json:"marcha_lenta_rpm"`
	IdleMarginRPM       float64  `json:"marcha_lenta_margem_rpm"`
	IdleFallbackRPM     float64  `json:"marcha_lenta_reserva_rpm"`
	EngineOnRPM         float64  `json:"motor_ligado_rpm"`
	MovingKmh           float64  `json:"movimento_kmh"`
	GPSGapS             float64  `json:"gps_falha_s"`
	SpeedMaxAgeMs       int64    `json:"velocidade_idade_max_ms"`
	HardAccelMS2        float64  `json:"acel_brusca_ms2"`
	HardBrakeMS2        float64  `json:"fren_brusca_ms2"`
	EventMinMs          int64    `json:"evento_min_ms"`
	LongIdleMs          int64    `json:"marcha_lenta_longa_ms"`
	StopMinS            float64  `json:"parada_min_s"`
	RateMinKm           float64  `json:"taxa_min_km"`
	PedalHighPct        float64  `json:"pedal_alto_pct"`
	PedalHighWarnFrac   float64  `json:"pedal_alto_aviso_fracao"`
	EngineBrakePedalPct float64  `json:"freio_motor_pedal_pct"`
	EngineBrakeRPM      float64  `json:"freio_motor_rpm"`
	EngineBrakeKmh      float64  `json:"freio_motor_kmh"`
	CruiseMinKmh        float64  `json:"cruzeiro_min_kmh"`
	CruiseAmplitudeKmh  float64  `json:"cruzeiro_amplitude_kmh"`
	CruiseMinS          float64  `json:"cruzeiro_min_s"`
	WarmCoolantC        float64  `json:"aquecido_c"`
	ColdCoolantC        float64  `json:"frio_c"`
	ColdRPM             float64  `json:"frio_rpm"`
	ColdPedalPct        float64  `json:"frio_pedal_pct"`
	SeriesMaxPoints     int      `json:"serie_max_pontos"`
}

// DefaultParams devolve os padrões da spec 02 (gasolina_e30).
func DefaultParams() Params {
	p := Params{
		IdleMarginRPM: 150, IdleFallbackRPM: 750, EngineOnRPM: 300, MovingKmh: 3, GPSGapS: 5, SpeedMaxAgeMs: 2000,
		HardAccelMS2: 3.0, HardBrakeMS2: -3.5, EventMinMs: 500, LongIdleMs: 60000, StopMinS: 5, RateMinKm: 20,
		PedalHighPct: 80, PedalHighWarnFrac: 0.10, EngineBrakePedalPct: 3, EngineBrakeRPM: 1200, EngineBrakeKmh: 15,
		CruiseMinKmh: 40, CruiseAmplitudeKmh: 10, CruiseMinS: 30,
		WarmCoolantC: 80, ColdCoolantC: 60, ColdRPM: 3000, ColdPedalPct: 60, SeriesMaxPoints: 5000,
	}
	p, _ = p.WithFuel("gasolina_e30")
	return p
}

// WithFuel troca o combustível. Os presets definem AFR e densidade; "personalizado" zera os dois e o chamador deve
// preenchê-los (Validate recusa se faltarem).
func (p Params) WithFuel(id string) (Params, error) {
	if id == FuelCustom {
		p.Fuel, p.AFR, p.DensityGL = id, 0, 0
		return p, nil
	}
	for _, f := range FuelPresets {
		if f.ID == id {
			p.Fuel, p.AFR, p.DensityGL = f.ID, f.AFR, f.DensityGL
			return p, nil
		}
	}
	return p, fmt.Errorf("%w: combustível %q", ErrInvalidParams, id)
}

// Validate confere os parâmetros que o cálculo usa como divisor ou limite.
func (p Params) Validate() error {
	switch {
	case p.Fuel == "":
		return fmt.Errorf("%w: combustível ausente", ErrInvalidParams)
	case p.AFR <= 0 || p.DensityGL <= 0:
		return fmt.Errorf("%w: afr e densidade_g_l são obrigatórios e positivos", ErrInvalidParams)
	case p.SeriesMaxPoints < 2:
		return fmt.Errorf("%w: serie_max_pontos", ErrInvalidParams)
	case p.MovingKmh < 0 || p.EngineOnRPM < 0 || p.GPSGapS <= 0:
		return fmt.Errorf("%w: limiares negativos", ErrInvalidParams)
	}
	return nil
}
