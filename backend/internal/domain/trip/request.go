package trip

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"tccelta/backend/internal/domain/tripanalysis"
)

// Erros da criação e da leitura da viagem.
var (
	// ErrInvalid é a base de FieldError: a entrada do POST /v1/viagens quebra uma regra.
	ErrInvalid = errors.New("entrada_invalida")
	// ErrDriverRepeated: já existe viagem (não falha) com o mesmo motorista na categoria.
	ErrDriverRepeated = errors.New("motorista_repetido")
	// ErrNotFound: a viagem não existe ou foi excluída.
	ErrNotFound = errors.New("viagem_desconhecida")
	// ErrNotReady: a análise só existe com a viagem pronta; NotReadyError traz o estado.
	ErrNotReady = errors.New("analise_indisponivel")
)

// NotReadyError é ErrNotReady com o status (pending, waiting_data ou failed) da viagem.
type NotReadyError struct{ Status string }

func (e NotReadyError) Error() string        { return ErrNotReady.Error() + ": " + e.Status }
func (e NotReadyError) Is(target error) bool { return target == ErrNotReady }

// FieldError diz qual campo da entrada quebrou qual regra.
type FieldError struct{ Field, Reason string }

func (e FieldError) Error() string        { return e.Field + ": " + e.Reason }
func (e FieldError) Is(target error) bool { return target == ErrInvalid }

// Tipos de entrada na API (entrada.tipo) e no banco (gold.trip.entry_type).
const (
	EntryTypeBoot     = "boot"
	EntryTypeInterval = "interval"
	apiEntryBoot      = "boot"
	apiEntryInterval  = "intervalo"
)

// Limites da entrada.
const (
	MaxDriverNameRunes = 60
	MaxCategoryRunes   = 40
	MaxIntervalSpan    = 7 * 24 * time.Hour
	maxBootID          = 65535
)

// Input é o pedido de criação como chegou, com ponteiros para distinguir campo ausente de zero. Os números são int64 para
// que a faixa seja conferida aqui, e não pelo decodificador.
type Input struct {
	DriverName, Category, Fuel string
	EntryType                  string // boot ou intervalo
	DongleID                   *int64
	BootID, BootGeneration     *int64
	T0, T1                     *string // RFC 3339 com fuso
}

// Draft é uma viagem validada e normalizada, pronta para ser inserida (o banco dá id e created_at).
type Draft struct {
	Category, DriverName, DriverNameNormalized string
	Fuel                                       string
	DongleID                                   int32
	EntryType                                  string
	BootGeneration, BootID                     *int32
	IntervalStart, IntervalEnd                 *time.Time
	TargetSeq                                  int64
}

// NormalizeName devolve o nome como o índice único o compara: sem espaços nas pontas e em minúsculas.
func NormalizeName(name string) string { return strings.ToLower(strings.TrimSpace(name)) }

// NewDraft valida a entrada e a normaliza. O erro é um FieldError.
func NewDraft(in Input) (Draft, error) {
	var d Draft
	name := strings.TrimSpace(in.DriverName)
	if err := checkName(name); err != nil {
		return d, err
	}
	cat := strings.TrimSpace(in.Category)
	if err := checkCategory(cat); err != nil {
		return d, err
	}
	if !validFuel(in.Fuel) {
		return d, FieldError{"combustivel", "deve ser um dos combustíveis predefinidos"}
	}
	d.DriverName, d.DriverNameNormalized, d.Category, d.Fuel = name, NormalizeName(name), strings.ToLower(cat), in.Fuel
	if in.DongleID == nil || *in.DongleID < 1 || *in.DongleID > math.MaxInt32 {
		return d, FieldError{"entrada.dongle_id", "obrigatório, inteiro maior ou igual a 1"}
	}
	d.DongleID = int32(*in.DongleID)
	switch in.EntryType {
	case apiEntryBoot:
		return d, d.bootEntry(in)
	case apiEntryInterval:
		return d, d.intervalEntry(in)
	}
	return d, FieldError{"entrada.tipo", "deve ser boot ou intervalo"}
}

func checkName(name string) error {
	n := utf8.RuneCountInString(name)
	if n < 1 || n > MaxDriverNameRunes {
		return FieldError{"motorista.nome", fmt.Sprintf("obrigatório, de 1 a %d caracteres", MaxDriverNameRunes)}
	}
	if strings.ContainsFunc(name, unicode.IsControl) {
		return FieldError{"motorista.nome", "não pode ter caracteres de controle"}
	}
	return nil
}

func checkCategory(cat string) error {
	n := utf8.RuneCountInString(cat)
	if n < 1 || n > MaxCategoryRunes {
		return FieldError{"categoria_viagem", fmt.Sprintf("obrigatória, de 1 a %d caracteres", MaxCategoryRunes)}
	}
	for _, r := range cat {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != ' ' && r != '_' && r != '-' {
			return FieldError{"categoria_viagem", "só letras, dígitos, espaço, _ e -"}
		}
	}
	return nil
}

func validFuel(id string) bool {
	for _, f := range tripanalysis.FuelPresets {
		if f.ID == id {
			return true
		}
	}
	return false
}

func (d *Draft) bootEntry(in Input) error {
	if in.T0 != nil || in.T1 != nil {
		return FieldError{"entrada", "t0 e t1 são da entrada intervalo"}
	}
	if in.BootID == nil || *in.BootID < 1 || *in.BootID > maxBootID {
		return FieldError{"entrada.boot_id", fmt.Sprintf("obrigatório, de 1 a %d", maxBootID)}
	}
	id := int32(*in.BootID)
	d.EntryType, d.BootID = EntryTypeBoot, &id
	if g := in.BootGeneration; g != nil {
		if *g < 0 || *g > maxBootID {
			return FieldError{"entrada.boot_generation", fmt.Sprintf("de 0 a %d", maxBootID)}
		}
		v := int32(*g)
		d.BootGeneration = &v
	}
	return nil
}

func (d *Draft) intervalEntry(in Input) error {
	if in.BootID != nil || in.BootGeneration != nil {
		return FieldError{"entrada", "boot_id e boot_generation são da entrada boot"}
	}
	t0, err := parseInstant("entrada.t0", in.T0)
	if err != nil {
		return err
	}
	t1, err := parseInstant("entrada.t1", in.T1)
	if err != nil {
		return err
	}
	if !t1.After(t0) {
		return FieldError{"entrada.t1", "deve ser posterior a t0"}
	}
	if t1.Sub(t0) > MaxIntervalSpan {
		return FieldError{"entrada.t1", "o intervalo vai até 7 dias"}
	}
	d.EntryType, d.IntervalStart, d.IntervalEnd = EntryTypeInterval, &t0, &t1
	return nil
}

// parseInstant lê RFC 3339 com fuso (Z ou ±hh:mm); data sem fuso é recusada pelo próprio layout.
func parseInstant(field string, s *string) (time.Time, error) {
	if s == nil {
		return time.Time{}, FieldError{field, "obrigatório"}
	}
	t, err := time.Parse(time.RFC3339, *s)
	if err != nil {
		return time.Time{}, FieldError{field, "deve ser RFC 3339 com fuso (Z ou ±hh:mm)"}
	}
	return t.UTC(), nil
}

// Analysis é o que a API lê da gold para montar o JSON da spec 02. Reaproveita os tipos do resultado do cálculo: o que o
// worker gravou é o que a API lê. Os campos jsonb vêm crus e são repassados.
type Analysis struct {
	Trip       Trip
	Parts      []tripanalysis.PartResult
	Summary    tripanalysis.Summary
	SpeedBands []tripanalysis.SpeedBand
	Events     []tripanalysis.Event
	Stops      []tripanalysis.Stop
	Gears      []tripanalysis.Gear
	GearShifts []tripanalysis.GearShift
	// Series guarda os jsonb de gold.trip_series (RawJSON) e os dois passos, que têm coluna própria.
	Series AnalysisSeries
	// Warnings e Params são os jsonb de gold.trip_summary.
	Warnings, Params RawJSON
}

// RawJSON é um valor JSON já serializado, repassado sem alteração.
type RawJSON []byte

// AnalysisSeries são as colunas de gold.trip_series.
type AnalysisSeries struct {
	StepMs, ExposureStepMs                           int32
	Series, Slow, Route                              RawJSON
	RPMDistribution, PedalDistribution, RPMPedalGrid RawJSON
}
