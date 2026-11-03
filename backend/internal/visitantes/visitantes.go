// Package visitantes e o contador de visitantes do site estatico. O valor vive em memoria e, se configurado um
// arquivo, e gravado nele (de forma assincrona) para sobreviver a restarts. Por isso o servico roda em uma unica
// instancia, separado das replicas da API.
package visitantes

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// Contador e seguro para uso concorrente.
type Contador struct {
	n     atomic.Uint64
	sujo  chan struct{} // sinaliza que ha valor novo a gravar
}

// Novo cria um contador comecando em inicial.
func Novo(inicial uint64) *Contador {
	c := &Contador{sujo: make(chan struct{}, 1)}
	c.n.Store(inicial)
	return c
}

// Carrega le o total gravado em caminho. Se o arquivo existe e e valido, ele vence o valor inicial; senao
// mantem o valor atual.
func (c *Contador) Carrega(caminho string) {
	b, err := os.ReadFile(caminho) //nolint:gosec // caminho vem da configuracao do operador
	if err != nil {
		return
	}
	if n, err := strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64); err == nil {
		c.n.Store(n)
	}
}

// Persiste grava o total em caminho (escrita atomica: temporario + rename) ate ctx terminar, no maximo uma vez
// por intervalo, e faz uma gravacao final ao sair.
func (c *Contador) Persiste(ctx context.Context, caminho string, intervalo time.Duration) {
	grava := func() {
		tmp := caminho + ".tmp"
		if os.WriteFile(tmp, []byte(strconv.FormatUint(c.Total(), 10)+"\n"), 0o600) == nil { //nolint:gosec
			_ = os.Rename(tmp, caminho)
		}
	}
	grava() // grava ja na subida (cria o arquivo); erros de gravacao sao ignorados (best effort)
	for {
		select {
		case <-ctx.Done():
			grava()
			return
		case <-c.sujo:
			grava()
			select {
			case <-ctx.Done():
			case <-time.After(intervalo):
			}
		}
	}
}

// Total devolve o valor atual.
func (c *Contador) Total() uint64 { return c.n.Load() }

// Incrementa soma um visitante e devolve o novo total.
func (c *Contador) Incrementa() uint64 {
	n := c.n.Add(1)
	select {
	case c.sujo <- struct{}{}:
	default:
	}
	return n
}

// Handler serve GET/POST /api/visitantes e GET /saude/viva.
func (c *Contador) Handler() http.Handler {
	mux := http.NewServeMux()
	resp := func(w http.ResponseWriter, total uint64) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]uint64{"total": total})
	}
	mux.HandleFunc("GET /api/visitantes", func(w http.ResponseWriter, _ *http.Request) { resp(w, c.Total()) })
	mux.HandleFunc("POST /api/visitantes", func(w http.ResponseWriter, _ *http.Request) { resp(w, c.Incrementa()) })
	mux.HandleFunc("GET /saude/viva", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	return mux
}
