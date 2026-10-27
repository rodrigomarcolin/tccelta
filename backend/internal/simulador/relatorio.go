package simulador

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"
)

// Relatorio acumula latências por rota e contagens por status.
type Relatorio struct {
	mu      sync.Mutex
	lat     map[string][]time.Duration
	Status  map[string]map[int]int
	Inicio  time.Time
	Cenario string
	Params  map[string]any
	Extras  map[string]any
}

// NovoRelatorio cria o relatório.
func NovoRelatorio(cenario string, params map[string]any) *Relatorio {
	return &Relatorio{lat: map[string][]time.Duration{}, Status: map[string]map[int]int{}, Inicio: time.Now(),
		Cenario: cenario, Params: params, Extras: map[string]any{}}
}

// Observar registra uma chamada.
func (r *Relatorio) Observar(rota string, status int, d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lat[rota] = append(r.lat[rota], d)
	if r.Status[rota] == nil {
		r.Status[rota] = map[int]int{}
	}
	r.Status[rota][status]++
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

// Resumo devolve o relatório em forma de mapa (JSON).
func (r *Relatorio) Resumo() map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	rotas := map[string]any{}
	total := 0
	for k, v := range r.lat {
		rotas[k] = map[string]any{"n": len(v), "p50_ms": pct(v, .5), "p95_ms": pct(v, .95), "p99_ms": pct(v, .99), "status": r.Status[k]}
		total += len(v)
	}
	dur := time.Since(r.Inicio).Seconds()
	out := map[string]any{"cenario": r.Cenario, "parametros": r.Params, "duracao_s": dur, "rotas": rotas,
		"requisicoes_por_s": float64(total) / max(dur, 0.001)}
	for k, v := range r.Extras {
		out[k] = v
	}
	return out
}

// Escrever imprime o resumo e grava o JSON.
func (r *Relatorio) Escrever(arq string) error {
	b, _ := json.MarshalIndent(r.Resumo(), "", "  ")
	fmt.Println(string(b))
	if arq == "" {
		return nil
	}
	return os.WriteFile(arq, b, 0o644)
}

// Cinco_xx conta respostas 5xx.
func (r *Relatorio) Cinco() int {
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
