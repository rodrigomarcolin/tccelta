package tripanalysis

import (
	"math"
	"sort"
	"testing"
	"time"
)

// gen gera uma série com uma leitura a cada period ms em [from, to] (inclusive).
func gen(pid uint8, from, to, period int64, f func(t int64) float64) PIDSeries {
	s := PIDSeries{PID: pid}
	for t := from; t <= to; t += period {
		s.Samples = append(s.Samples, Sample{TMs: t, Value: f(t)})
	}
	return s
}

func konst(v float64) func(int64) float64 { return func(int64) float64 { return v } }

// readings ordena as séries por PID, como o carregador faz.
func readings(ss ...PIDSeries) []PIDSeries {
	sort.Slice(ss, func(i, j int) bool { return ss[i].PID < ss[j].PID })
	return ss
}

func i32p(v int32) *int32 { return &v }

func bootEntry() Entry {
	return Entry{Type: EntryBoot, DongleID: 1, BootGeneration: i32p(0), BootID: i32p(1)}
}

func input(parts ...PartInput) Input {
	return Input{Entry: bootEntry(), Parts: parts, Params: DefaultParams()}
}

func mustAnalyze(t *testing.T, in Input) Result {
	t.Helper()
	r, err := Analyze(in)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func near(t *testing.T, name string, got *float64, want, tol float64) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s: nulo, esperado %v", name, want)
	}
	if math.Abs(*got-want) > tol {
		t.Fatalf("%s: %v, esperado %v (+-%v)", name, *got, want, tol)
	}
}

func isNil(t *testing.T, name string, got *float64) {
	t.Helper()
	if got != nil {
		t.Fatalf("%s: %v, esperado nulo", name, *got)
	}
}

func hasWarning(r Result, code string) bool {
	for _, w := range r.Warnings {
		if w.Code == code {
			return true
		}
	}
	return false
}

func warningCodes(r Result) []string {
	var out []string
	for _, w := range r.Warnings {
		out = append(out, w.Code)
	}
	return out
}

// simple monta uma parte de n segundos (uma leitura por segundo) com rpm e velocidade constantes.
func simple(boot int32, from, n int64, speed float64) PartInput {
	to := from + (n-1)*1000
	return PartInput{BootID: boot, Readings: readings(
		gen(pidRPM, from, to, 1000, konst(1500)),
		gen(pidSpeed, from, to, 1000, konst(speed)))}
}

var t0 = time.Date(2026, 10, 28, 11, 0, 0, 0, time.UTC)
