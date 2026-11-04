package tripanalysis

import "time"

// Result espelha as tabelas da gold. Ausência de valor é nil (nunca zero). Os json são os nomes do contrato nas
// estruturas que vão para colunas jsonb.
type Result struct {
	Parts      []PartResult
	Summary    Summary
	SpeedBands []SpeedBand
	Events     []Event
	Stops      []Stop
	Gears      []Gear
	GearShifts []GearShift
	Series     SeriesData
	Warnings   []Warning
	Params     Params
}

// PartResult é uma parte da viagem. StartTMs/EndTMs são nulos se a parte não teve motor ligado.
type PartResult struct {
	BootGeneration, BootID   int32
	DataStartTMs, DataEndTMs int64
	StartTMs, EndTMs         *int64
	Clock                    *Clock
}

// Summary agrupa resumo, condutor, motor, marchas, qualidade e relógio.
type Summary struct {
	StartTMs, EndTMs                                  *int64
	StartedAt, EndedAt                                *time.Time
	ClockOffsetMs                                     *int64
	ClockUncertaintyMs                                *int32
	DistanceKm, DurationS, NoDataS, MovingS, StoppedS *float64
	AvgSpeedKmh, MaxSpeedKmh                          *float64
	EngineOnS, IdleS, IdlePct                         *float64
	FuelL, KmPerL, IntakeAirG                         *float64
	NHardAccel, NHardBrake                            *int32
	HardAccelPer100Km, HardBrakePer100Km              *float64
	PedalMeanPct, PedalHighS, PedalRateP95PctS        *float64
	EngineBrakePct, AccelRMSMS2, CruiseS              *float64
	WarmupS, MaxCoolantC, ColdLoadS                   *float64
	IdleRPM                                           *float64
	IdleRPMOrigin                                     *string
	IdleMarginRPM                                     *float64
	NeutralClutchS                                    *float64
	UpshiftRPMMedian, UpshiftRPMP25, UpshiftRPMP75    *float64
	UpshiftN                                          *int32
	DownshiftRPMMedian                                *float64
	DownshiftN                                        *int32
	NReadings                                         int64
	ReadingsPerS                                      *float64
	PIDPeriodsMs                                      map[string]float64
	FuelCoveragePct                                   *float64
	GPSPeriodS, GPSCoveragePct                        *float64
	GPSNGaps                                          *int32
	GPSMaxGapS, GPSDistanceKm, GPSDistanceDiffPct     *float64
}

// SpeedBand é uma faixa de velocidade.
type SpeedBand struct {
	ID          string
	FromKmh     float64
	ToKmh       *float64
	TimeS       float64
	DistanceKm  float64
	DistancePct *float64
}

// Tipos de evento.
const (
	EventHardAccel = "aceleracao_brusca"
	EventHardBrake = "frenagem_brusca"
	EventLongIdle  = "marcha_lenta_longa"
)

// Event é um evento brusco ou uma marcha lenta longa.
type Event struct {
	Type                       string
	StartTMs                   int64
	DurationS                  float64
	Peak                       float64
	PeakUnit                   string
	StartSpeedKmh, EndSpeedKmh *float64
	Lat, Lon                   *float64
}

// Stop é uma parada.
type Stop struct {
	StartTMs  int64
	DurationS float64
	EngineOn  bool
	Lat, Lon  *float64
}

// Gear é um patamar de marcha detectado.
type Gear struct {
	Gear      int
	RPMPerKmh float64
	TimeS     float64
	TimePct   float64
}

// GearShift é uma troca de marcha.
type GearShift struct {
	TMs                 int64
	Direction           string
	From, To            int
	RPMBefore, RPMAfter float64
	SpeedKmh            float64
}

// Sentidos da troca.
const (
	ShiftUp   = "subida"
	ShiftDown = "reducao"
)

// SeriesData reúne o que vai para gold.trip_series.
type SeriesData struct {
	StepMs, ExposureStepMs int32
	Series                 Series      // coluna series (sem os dois passos)
	Slow                   SlowSeries  // series_lentas
	Route                  Route       // rota
	RPMDistribution        []RPMBand   // distribuicoes.rotacao
	PedalDistribution      []PedalBand // distribuicoes.pedal
	RPMPedal               RPMPedalMatrix
}

// Series são as séries da grade, em arrays paralelos. Entre partes há um ponto separador com valores nulos.
type Series struct {
	TMs       []int64    `json:"t_ms"`
	Speed     []*float64 `json:"velocidade_kmh"`
	RPM       []*float64 `json:"rpm"`
	RPMPerKmh []*float64 `json:"rpm_por_kmh"`
	Gear      []*int     `json:"marcha"`
	Pedal     []*float64 `json:"pedal_pct"`
	Accel     []*float64 `json:"aceleracao_ms2"`
	Load      []*float64 `json:"carga_pct"`
	FuelLH    []*float64 `json:"consumo_l_h"`
	FuelCumL  []*float64 `json:"combustivel_acumulado_l"`
	DistCumKm []*float64 `json:"distancia_acumulada_km"`
}

// SlowSeries são a temperatura e o nível do tanque (leituras brutas).
type SlowSeries struct {
	Temperature TemperatureSeries `json:"temperatura"`
	Tank        TankSeries        `json:"nivel_tanque"`
}

// TemperatureSeries é o líquido de arrefecimento.
type TemperatureSeries struct {
	TMs []int64    `json:"t_ms"`
	C   []*float64 `json:"c"`
}

// TankSeries é o nível do tanque com a mediana móvel de ±3 leituras.
type TankSeries struct {
	TMs       []int64    `json:"t_ms"`
	Pct       []*float64 `json:"pct"`
	MedianPct []*float64 `json:"mediana_movel_pct"`
}

// Route é a rota colorida pela velocidade OBD.
type Route struct {
	TMs      []int64       `json:"t_ms"`
	Lat      []float64     `json:"lat"`
	Lon      []float64     `json:"lon"`
	Segments RouteSegments `json:"segmentos"`
}

// RouteSegments tem um item por par de pontos consecutivos.
type RouteSegments struct {
	SpeedKmh []*float64 `json:"velocidade_kmh"`
	Band     []string   `json:"faixa"`
}

// Faixas de segmento sem velocidade de faixa.
const (
	BandGPSFailure = "falha_gps"
	BandNoSpeed    = "sem_velocidade"
)

// RPMBand é uma faixa de rotação.
type RPMBand struct {
	From  float64  `json:"de"`
	To    *float64 `json:"ate"`
	TimeS float64  `json:"tempo_s"`
	Pct   float64  `json:"pct"`
}

// PedalBand é uma faixa de pedal.
type PedalBand struct {
	From  float64 `json:"de"`
	To    float64 `json:"ate"`
	Label *string `json:"rotulo,omitempty"`
	TimeS float64 `json:"tempo_s"`
	Pct   float64 `json:"pct"`
}

// RPMPedalMatrix é o tempo em células de 500 rpm x 10% de pedal.
type RPMPedalMatrix struct {
	RPMBin   int         `json:"rpm_bin"`
	PedalBin int         `json:"pedal_bin"`
	TimeS    [][]float64 `json:"tempo_s"`
}
