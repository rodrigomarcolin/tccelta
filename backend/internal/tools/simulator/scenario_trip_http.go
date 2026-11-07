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
// status até ficar pronta e confere a análise. Casos: (1) boot já enviado, que vira viagem pronta; (2) a mesma viagem de novo, que a API
// recusa com 409 viagem_repetida (e, em outro boot, o mesmo motorista com 409 motorista_repetido); (3) a listagem GET /v1/viagens, com busca
// e cursor; (4) boot inexistente, que a API recusa com 404 boot_desconhecido; (5) a exclusão, que tira a viagem de tudo e libera o nome e a entrada. O dongle tem dois boots (uma queda de energia depois dos
// primeiros blocos), para o nome repetido poder ser testado sem repetir a viagem. Exige --client
// (client credentials): usa o Bearer, como o servidor do Next.js. O worker precisa das esperas curtas do compose de teste.
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
	// Segundo boot: queda de energia e mais alguns blocos. A viagem 1 usa o primeiro boot e o nome repetido é testado no segundo.
	firstBoot := est.LastBootID
	d.LosePower(3)
	if err := a.Handshake(ctx); err != nil {
		return fmt.Errorf("handshake após a queda: %w", err)
	}
	if err := generateAndDrain(ctx, a, 40); err != nil {
		return fmt.Errorf("blocos do segundo boot: %w", err)
	}
	if est, err = q.DongleState(ctx, ch.Dev); err != nil {
		return err
	}
	secondBoot := est.LastBootID
	v.ok(secondBoot != firstBoot, "dois boots: %d e %d", firstBoot, secondBoot)
	waitForBoots(ctx, q, est.DongleID, 15*time.Second)
	api := &tripAPI{base: o.API, http: client()}
	// As rotas de viagem exigem o Bearer: obtém o token de client credentials, como o servidor do Next.js.
	cid, csecret, err := primaryClient(o)
	if err != nil {
		return fmt.Errorf("trip-http exige --client: %w", err)
	}
	if api.token, err = fetchToken(ctx, api.http, o.API, cid, csecret); err != nil {
		return err
	}
	st, body, _ := api.do(ctx, "GET", "/v1/viagens/6f1c1f7e-3f0a-4a53-9d51-0c1d7a1b2c3d", "")
	v.ok(st == 404 && jstr(body, "codigo") == "viagem_desconhecida", "token de cliente aceito nas rotas de viagem: %d %s", st, jstr(body, "codigo"))
	category := fmt.Sprintf("e2e-http-%d", time.Now().Unix()) // o nome do motorista é único por categoria; cada execução usa a sua

	// erros de contrato, antes de criar viagens
	st, body, _ = api.do(ctx, "POST", "/v1/viagens", tripBody("Erro", category, est.DongleID+1000, firstBoot))
	v.ok(st == 404 && jstr(body, "codigo") == "dongle_desconhecido", "dongle sem estado: %d %s", st, jstr(body, "codigo"))
	st, body, _ = api.do(ctx, "POST", "/v1/viagens", strings.Replace(tripBody("Erro", category, est.DongleID, firstBoot), "gasolina_e30", "personalizado", 1))
	v.ok(st == 400 && jstr(body, "codigo") == "entrada_invalida", "combustível personalizado: %d %s", st, jstr(body, "codigo"))
	st, _, _ = api.do(ctx, "GET", "/v1/viagens/abc", "")
	v.ok(st == 404, "GET com id que não é UUID: %d", st)

	// 1. boot já enviado
	id, loc, err := api.create(ctx, tripBody("Boot enviado", category, est.DongleID, firstBoot))
	if err != nil {
		return err
	}
	v.ok(loc == "/v1/viagens/"+id, "Location %q", loc)
	st, body, _ = api.do(ctx, "POST", "/v1/viagens", tripBody("boot ENVIADO ", category, est.DongleID, secondBoot))
	v.ok(st == 409 && jstr(body, "codigo") == "motorista_repetido", "mesmo motorista com outra caixa, em outro boot: %d %s", st, jstr(body, "codigo"))
	status := api.waitReady(ctx, id, 40*time.Second)
	v.ok(jstr(status, "status") == "pronta", "viagem 1 pronta (status %q)", jstr(status, "status"))
	// A mesma viagem de novo: outro motorista, o mesmo nome (viagem_repetida vence motorista_repetido), outra categoria e a geração explícita.
	generation := int(jnum(status, "entrada", "boot_generation"))
	repeats := []struct{ name, body string }{
		{"outro motorista", tripBody("Outro motorista", category, est.DongleID, firstBoot)},
		{"mesmo motorista", tripBody("boot ENVIADO ", category, est.DongleID, firstBoot)},
		{"outra categoria", tripBody("Outro motorista", category+"-b", est.DongleID, firstBoot)},
		{"geração explícita", strings.Replace(tripBody("Outro motorista", category, est.DongleID, firstBoot), `"boot_id"`,
			fmt.Sprintf(`"boot_generation":%d,"boot_id"`, generation), 1)},
	}
	for _, r := range repeats {
		st, resp, _ := api.do(ctx, "POST", "/v1/viagens", r.body)
		v.ok(st == 409 && jstr(resp, "codigo") == "viagem_repetida" && jstr(resp, "viagem_id") == id && jstr(resp, "motorista", "nome") == "Boot enviado",
			"viagem repetida (%s): %d %s viagem_id=%s motorista=%q", r.name, st, jstr(resp, "codigo"), jstr(resp, "viagem_id"), jstr(resp, "motorista", "nome"))
	}
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

	// 2. listagem: duas viagens com o mesmo trecho no nome, lidas de uma em uma por cursor, da mais nova para a mais antiga
	id2, _, err := api.create(ctx, tripBody("Boot enviado 2", category, est.DongleID, secondBoot))
	if err != nil {
		return err
	}
	api.waitReady(ctx, id2, 40*time.Second)
	st, body, hdr := api.do(ctx, "GET", "/v1/viagens?limite=1&busca=%20Boot%20ENVIADO", "")
	next := jstr(body, "proximo_cursor")
	v.ok(st == 200 && hdr.Get("Cache-Control") == "no-store" && jnum(body, "total") == 2 && jlen(body, "viagens") == 1 && next != "" &&
		jstr(listItem(body, 0), "viagem_id") == id2, "listagem, página 1: %d total=%v itens=%d cursor=%q", st, jraw(body, "total"), jlen(body, "viagens"), next)
	st, body, _ = api.do(ctx, "GET", "/v1/viagens?limite=1&busca=boot%20enviado&cursor="+next, "")
	item := listItem(body, 0)
	v.ok(st == 200 && jnum(body, "total") == 2 && jlen(body, "viagens") == 1 && jstr(item, "viagem_id") == id && jraw(body, "proximo_cursor") == nil,
		"listagem, página 2 é a última e traz a viagem 1: %d %v", st, jraw(body, "proximo_cursor"))
	v.ok(jstr(item, "status") == "pronta" && jstr(item, "motorista", "nome") == "Boot enviado" && jstr(item, "categoria_viagem") == category &&
		jstr(item, "entrada_tipo") == "boot" && jnum(item, "dongle_id") == float64(est.DongleID) && jnum(item, "boot_id") == float64(firstBoot) &&
		jraw(item, "boot_generation") != nil && jstr(item, "inicio_em") != "" && jstr(item, "fim_em") != "" && jnum(item, "distancia_km") > 0 &&
		jnum(item, "duracao_s") > 0 && jstr(item, "criada_em") != "" && jlen(item, "rota_resumo") >= 1,
		"listagem, campos da viagem 1: %v", item)
	st, body, _ = api.do(ctx, "GET", "/v1/viagens?busca=nome-que-ninguem-tem", "")
	v.ok(st == 200 && jnum(body, "total") == 0 && jraw(body, "proximo_cursor") == nil && jraw(body, "viagens") != nil && jlen(body, "viagens") == 0,
		"listagem sem resultado: %d %v", st, body)
	for _, bad := range []string{"limite=0", "limite=51", "limite=abc", "cursor=lixo", "busca=" + strings.Repeat("a", 61)} {
		st, body, _ = api.do(ctx, "GET", "/v1/viagens?"+bad, "")
		v.ok(st == 400 && jstr(body, "codigo") == "entrada_invalida", "listagem com %s: %d %s", bad, st, jstr(body, "codigo"))
	}
	st, body, _ = (&tripAPI{base: o.API, http: api.http}).do(ctx, "GET", "/v1/viagens", "")
	v.ok(st == 401, "listagem sem Bearer: %d %s", st, jstr(body, "codigo"))

	// 3. boot que nunca chegou ao servidor (o próximo, ainda não enviado, e um número absurdo): a API recusa com 404, sem criar
	// viagem vazia que ficaria "aguardando dados" para sempre.
	for _, bootID := range []int32{est.LastBootID + 1000, 40000} {
		st, body, _ = api.do(ctx, "POST", "/v1/viagens", tripBody(fmt.Sprintf("Boot ausente %d", bootID), category, est.DongleID, bootID))
		v.ok(st == 404 && jstr(body, "codigo") == "boot_desconhecido", "boot %d inexistente: %d %s", bootID, st, jstr(body, "codigo"))
	}

	// 4. exclusão (EXC1): a viagem 2 some do status, da análise e da listagem, uma segunda exclusão dá 404 e a mesma entrada com o
	// mesmo nome volta a ser aceita (202, não 409).
	st, body, _ = api.do(ctx, "DELETE", "/v1/viagens/"+id2, "")
	v.ok(st == 204 && body == nil, "exclusão da viagem 2: %d", st)
	for _, path := range []string{"/v1/viagens/" + id2, "/v1/viagens/" + id2 + "/analise"} {
		st, body, _ = api.do(ctx, "GET", path, "")
		v.ok(st == 404 && jstr(body, "codigo") == "viagem_desconhecida", "GET %s da excluída: %d %s", path, st, jstr(body, "codigo"))
	}
	st, body, _ = api.do(ctx, "DELETE", "/v1/viagens/"+id2, "")
	v.ok(st == 404 && jstr(body, "codigo") == "viagem_desconhecida", "segunda exclusão: %d %s", st, jstr(body, "codigo"))
	st, body, _ = api.do(ctx, "GET", "/v1/viagens?busca=boot%20enviado", "")
	v.ok(st == 200 && jnum(body, "total") == 1 && jlen(body, "viagens") == 1 && jstr(listItem(body, 0), "viagem_id") == id,
		"listagem sem a excluída: %d total=%v", st, jraw(body, "total"))
	st, body, _ = (&tripAPI{base: o.API, http: api.http}).do(ctx, "DELETE", "/v1/viagens/"+id, "")
	v.ok(st == 401, "exclusão sem Bearer: %d %s", st, jstr(body, "codigo"))
	id3, _, err := api.create(ctx, tripBody("Boot enviado 2", category, est.DongleID, secondBoot))
	v.ok(err == nil && id3 != id2, "mesma entrada e mesmo nome depois de excluir: 202 (viagem %s, erro %v)", id3, err)
	return v.result()
}

func tripBody(driver, category string, dongleID int32, bootID int32) string {
	return fmt.Sprintf(`{"motorista":{"nome":%q},"categoria_viagem":%q,"combustivel":"gasolina_e30","entrada":{"tipo":"boot","dongle_id":%d,"boot_id":%d}}`,
		driver, category, dongleID, bootID)
}

// tripAPI é o cliente HTTP das rotas de viagem.
type tripAPI struct {
	base  string
	http  *http.Client
	token string // Bearer de client credentials
}

func (t *tripAPI) do(ctx context.Context, method, path, body string) (int, map[string]any, http.Header) {
	req, err := http.NewRequestWithContext(ctx, method, t.base+path, bytes.NewBufferString(body))
	if err != nil {
		return 0, nil, nil
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if t.token != "" {
		req.Header.Set("Authorization", "Bearer "+t.token)
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

func jstr(m map[string]any, path ...string) string { s, _ := jraw(m, path...).(string); return s }
func jnum(m map[string]any, path ...string) float64 {
	f, _ := jraw(m, path...).(float64)
	return f
}
func jlen(m map[string]any, path ...string) int { l, _ := jraw(m, path...).([]any); return len(l) }

// listItem devolve o i-ésimo item de "viagens" da listagem (mapa vazio se não existe).
func listItem(m map[string]any, i int) map[string]any {
	l, _ := jraw(m, "viagens").([]any)
	if i >= len(l) {
		return map[string]any{}
	}
	x, _ := l[i].(map[string]any)
	return x
}
