package simulator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// tripHTTPScenario passa pela API de viagens (API2), sem tocar em gold.trip: cria a viagem por POST /v1/viagens, consulta o
// status até ficar pronta e confere a análise. Três casos: (1) boot já enviado; (2) boot pedido antes de chegar, com a
// geração resolvida pelo worker; (3) boot que nunca chega, que fecha provisório e vazio. A API precisa estar com
// ANALISE_SEM_AUTH=true (compose.test.yaml), e o worker, com as esperas curtas do compose de teste.
func tripHTTPScenario(ctx context.Context, o Options) error {
	v := &verifier{log: o.Log}
	q := o.Inspect
	ch := o.Keys[0]
	d := NewDongle(ch, ConfigDongle{FlashCapacity: 400})
	a := NewApp(o.API, d, client(), o.Report)
	if err := a.Handshake(ctx); err != nil {
		return fmt.Errorf("handshake: %w", err)
	}
	if err := generateAndDrain(ctx, a, 200); err != nil {
		return fmt.Errorf("blocos: %w", err)
	}
	est, err := q.DongleState(ctx, ch.Dev)
	if err != nil {
		return err
	}
	waitForBoots(ctx, q, est.DongleID, 15*time.Second)
	api := &tripAPI{base: o.API, http: client()}
	category := fmt.Sprintf("e2e-http-%d", time.Now().Unix()) // o nome do motorista é único por categoria; cada execução usa a sua

	// erros de contrato, antes de criar viagens
	st, body, _ := api.do(ctx, "POST", "/v1/viagens", tripBody("Erro", category, est.DongleID+1000, est.LastBootID))
	v.ok(st == 404 && jstr(body, "codigo") == "dongle_desconhecido", "dongle sem estado: %d %s", st, jstr(body, "codigo"))
	st, body, _ = api.do(ctx, "POST", "/v1/viagens", strings.Replace(tripBody("Erro", category, est.DongleID, est.LastBootID), "gasolina_e30", "personalizado", 1))
	v.ok(st == 400 && jstr(body, "codigo") == "entrada_invalida", "combustível personalizado: %d %s", st, jstr(body, "codigo"))
	st, _, _ = api.do(ctx, "GET", "/v1/viagens/abc", "")
	v.ok(st == 404, "GET com id que não é UUID: %d", st)

	// 1. boot já enviado
	id, loc, err := api.create(ctx, tripBody("Boot enviado", category, est.DongleID, est.LastBootID))
	if err != nil {
		return err
	}
	v.ok(loc == "/v1/viagens/"+id, "Location %q", loc)
	st, body, _ = api.do(ctx, "POST", "/v1/viagens", tripBody("boot ENVIADO ", category, est.DongleID, est.LastBootID))
	v.ok(st == 409 && jstr(body, "codigo") == "motorista_repetido", "mesmo motorista com outra caixa: %d %s", st, jstr(body, "codigo"))
	status := api.waitReady(ctx, id, 40*time.Second)
	v.ok(jstr(status, "status") == "pronta", "viagem 1 pronta (status %q)", jstr(status, "status"))
	v.ok(jraw(status, "entrada", "boot_generation") != nil,
		"viagem 1 com boot_generation resolvida: %v", jraw(status, "entrada", "boot_generation"))
	v.ok(jraw(status, "calculada_em") != nil, "viagem 1 com calculada_em")
	an := api.analysis(ctx, id)
	v.ok(jstr(an, "versao_calculo") != "", "versao_calculo %q", jstr(an, "versao_calculo"))
	v.ok(jstr(an, "viagem", "motorista", "nome") == "Boot enviado" && jstr(an, "viagem", "motorista", "id") == id &&
		jstr(an, "viagem", "categoria_viagem") == category, "viagem.motorista e categoria_viagem")
	v.ok(jnum(an, "resumo", "distancia_km") > 0 && jnum(an, "resumo", "motor_ligado_s") > 0, "distância %.4f km, motor ligado %.1f s",
		jnum(an, "resumo", "distancia_km"), jnum(an, "resumo", "motor_ligado_s"))
	v.ok(jlen(an, "viagem", "partes") >= 1 && jlen(an, "series", "t_ms") > 1 && jnum(an, "series", "passo_ms") > 0 && jraw(an, "rota") != nil,
		"partes %d, pontos %d, passo %.0f ms, rota presente", jlen(an, "viagem", "partes"), jlen(an, "series", "t_ms"), jnum(an, "series", "passo_ms"))
	v.ok(jstr(an, "parametros", "combustivel") == "gasolina_e30", "parametros.combustivel %q", jstr(an, "parametros", "combustivel"))

	// 2. boot pedido antes de chegar: a viagem nasce com boot_generation nula e o worker a resolve. O POST sai logo depois de um lote,
	// com o worker ainda esperando; o boot novo só aparece no servidor no handshake seguinte.
	futureBoot := int32(d.bootIDNow()) + 1
	if err := generateAndDrain(ctx, a, 5); err != nil { // um lote recente: o worker só fecha a viagem depois de um silêncio
		return err
	}
	id2, _, err := api.create(ctx, tripBody("Boot futuro", category, est.DongleID, futureBoot))
	if err != nil {
		return err
	}
	status = api.status(ctx, id2)
	v.ok(jhas(status, "entrada", "boot_generation") && jraw(status, "entrada", "boot_generation") == nil,
		"viagem 2 nasce com boot_generation nula")
	d.LosePower(0)
	d.GenerateBlocks(150)
	if err := a.Handshake(ctx); err != nil {
		return fmt.Errorf("handshake do boot futuro: %w", err)
	}
	if err := a.Drain(ctx); err != nil {
		return fmt.Errorf("blocos do boot futuro: %w", err)
	}
	status = api.waitReady(ctx, id2, 40*time.Second)
	v.ok(jstr(status, "status") == "pronta" && jraw(status, "entrada", "boot_generation") != nil, "viagem 2 pronta com a geração resolvida (%v)",
		jraw(status, "entrada", "boot_generation"))
	an = api.analysis(ctx, id2)
	v.ok(jnum(an, "resumo", "distancia_km") > 0 && jlen(an, "viagem", "partes") >= 1, "viagem 2 com dados: distância %.4f km", jnum(an, "resumo", "distancia_km"))

	// 3. boot que nunca chega: fecha provisória e vazia no limite final
	id3, _, err := api.create(ctx, tripBody("Boot ausente", category, est.DongleID, 40000))
	if err != nil {
		return err
	}
	st, body, _ = api.do(ctx, "GET", "/v1/viagens/"+id3+"/analise", "")
	v.ok(st == 409 && jstr(body, "codigo") == "analise_indisponivel" && jstr(body, "viagem_status") != "pronta",
		"análise de viagem não pronta: %d %s (%s)", st, jstr(body, "codigo"), jstr(body, "viagem_status"))
	status = api.waitReady(ctx, id3, 40*time.Second)
	v.ok(jstr(status, "status") == "pronta" && jbool(status, "provisoria"), "viagem 3 pronta e provisória")
	an = api.analysis(ctx, id3)
	v.ok(hasWarning(an, "sem_leituras"), "viagem 3 com o aviso sem_leituras")
	v.ok(jraw(an, "resumo", "distancia_km") == nil && jraw(an, "rota") == nil && jlen(an, "eventos") == 0 && jlen(an, "paradas") == 0 &&
		jlen(an, "faixas_velocidade") == 0, "viagem 3 vazia: distância e rota nulas, listas vazias")
	return v.result()
}

func tripBody(driver, category string, dongleID int32, bootID int32) string {
	return fmt.Sprintf(`{"motorista":{"nome":%q},"categoria_viagem":%q,"combustivel":"gasolina_e30","entrada":{"tipo":"boot","dongle_id":%d,"boot_id":%d}}`,
		driver, category, dongleID, bootID)
}

// tripAPI é o cliente HTTP das rotas de viagem (sem autenticação).
type tripAPI struct {
	base string
	http *http.Client
}

func (t *tripAPI) do(ctx context.Context, method, path, body string) (int, map[string]any, http.Header) {
	req, err := http.NewRequestWithContext(ctx, method, t.base+path, bytes.NewBufferString(body))
	if err != nil {
		return 0, nil, nil
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := t.http.Do(req)
	if err != nil {
		return 0, nil, nil
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	return resp.StatusCode, m, resp.Header
}

// create faz o POST e devolve o viagem_id e o Location; erro se a resposta não for 202.
func (t *tripAPI) create(ctx context.Context, body string) (id, location string, err error) {
	st, m, h := t.do(ctx, "POST", "/v1/viagens", body)
	if st != http.StatusAccepted {
		return "", "", fmt.Errorf("POST /v1/viagens: %d %v", st, m)
	}
	return jstr(m, "viagem_id"), h.Get("Location"), nil
}

func (t *tripAPI) status(ctx context.Context, id string) map[string]any {
	_, m, _ := t.do(ctx, "GET", "/v1/viagens/"+id, "")
	return m
}

// waitReady consulta o status a cada segundo até pronta ou falhou (ou o prazo acabar) e devolve o último corpo.
func (t *tripAPI) waitReady(ctx context.Context, id string, maxWait time.Duration) map[string]any {
	end := time.Now().Add(maxWait)
	for {
		m := t.status(ctx, id)
		if s := jstr(m, "status"); s == "pronta" || s == "falhou" || time.Now().After(end) || ctx.Err() != nil {
			return m
		}
		time.Sleep(time.Second)
	}
}

func (t *tripAPI) analysis(ctx context.Context, id string) map[string]any {
	_, m, _ := t.do(ctx, "GET", "/v1/viagens/"+id+"/analise", "")
	return m
}

// jraw navega pelo caminho de chaves e devolve o valor (nil se ausente ou null).
func jraw(m map[string]any, path ...string) any {
	var cur any = m
	for _, k := range path {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = mm[k]
	}
	return cur
}

func jhas(m map[string]any, path ...string) bool {
	parent := jraw(m, path[:len(path)-1]...)
	if len(path) == 1 {
		parent = m
	}
	mm, ok := parent.(map[string]any)
	if !ok {
		return false
	}
	_, ok = mm[path[len(path)-1]]
	return ok
}

func jstr(m map[string]any, path ...string) string { s, _ := jraw(m, path...).(string); return s }
func jnum(m map[string]any, path ...string) float64 {
	f, _ := jraw(m, path...).(float64)
	return f
}
func jbool(m map[string]any, path ...string) bool { b, _ := jraw(m, path...).(bool); return b }
func jlen(m map[string]any, path ...string) int   { l, _ := jraw(m, path...).([]any); return len(l) }

func hasWarning(an map[string]any, code string) bool {
	ws, _ := jraw(an, "avisos").([]any)
	for _, w := range ws {
		if mm, ok := w.(map[string]any); ok && mm["codigo"] == code {
			return true
		}
	}
	return false
}
