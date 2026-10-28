package simulator

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"
)

// Report acumula latências por rota e contagens por status.
type Report struct {
	mu       sync.Mutex
	lat      map[string][]time.Duration
	Status   map[string]map[int]int
	Start    time.Time
	Scenario string
	Params   map[string]any
	Extra    map[string]any
}

// NewReport cria o relatório.
func NewReport(scenario string, params map[string]any) *Report {
	return &Report{lat: map[string][]time.Duration{}, Status: map[string]map[int]int{}, Start: time.Now(),
		Scenario: scenario, Params: params, Extra: map[string]any{}}
}

// Observe registra uma chamada.
func (r *Report) Observe(route string, status int, d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lat[route] = append(r.lat[route], d)
	if r.Status[route] == nil {
		r.Status[route] = map[int]int{}
	}
	r.Status[route][status]++
}

func pct(v []time.Duration, p float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]time.Duration(nil), v...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	i := int(float64(len(s)-1) * p)
	return float64(s[i].Microseconds()) / 1000
}

// Summary devolve o relatório em forma de mapa (JSON).
func (r *Report) Summary() map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	routes := map[string]any{}
	total := 0
	for k, v := range r.lat {
		routes[k] = map[string]any{"n": len(v), "p50_ms": pct(v, .5), "p95_ms": pct(v, .95), "p99_ms": pct(v, .99), "status": r.Status[k]}
		total += len(v)
	}
	dur := time.Since(r.Start).Seconds()
	out := map[string]any{"scenario": r.Scenario, "params": r.Params, "duration_s": dur, "routes": routes,
		"requests_per_s": float64(total) / max(dur, 0.001)}
	for k, v := range r.Extra {
		out[k] = v
	}
	return out
}

// Write imprime o resumo e grava o JSON.
func (r *Report) Write(file string) error {
	b, _ := json.MarshalIndent(r.Summary(), "", "  ")
	fmt.Println(string(b))
	if file == "" {
		return nil
	}
	return os.WriteFile(file, b, 0o600)
}

// Cinco_xx conta respostas 5xx.
func (r *Report) Count5xx() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, m := range r.Status {
		for s, c := range m {
			if s >= 500 {
				n += c
			}
		}
	}
	return n
}
