# API2 — Endpoints de viagem

> **Status: especificação de algo a desenvolver.** Esta entrega expõe a criação, o status e a análise de uma viagem, sem autenticação. As decisões de arquitetura estão em [`04-arquitetura.md`](../04-arquitetura.md) e o JSON em [`02-calculos-e-contrato-json.md`](../02-calculos-e-contrato-json.md). Aqui entra só o que é novo.

## Objetivo

Três rotas HTTP em `backend/`, ligadas ao worker da API1 por `gold.trip` e `NOTIFY trip_pending`. A API valida, congela o ponto de corte dos dados e lê a gold. Ela não calcula nada.

## Escopo e fora de escopo

| Dentro | Fora |
|---|---|
| `POST /v1/viagens`, `GET /v1/viagens/{id}`, `GET /v1/viagens/{id}/analise` | `DELETE`, lista de boots, ranking |
| Modo `ANALISE_SEM_AUTH` | Bearer, client credentials, JWKS (API3) |
| Análise só com parâmetros padrão e o combustível da viagem | `de_t_ms`, `ate_t_ms` e parâmetros de Ajustes |
| Permissões do `tccelta_api` na gold | CORS (ver "Middlewares") |

## Dependências

- **API1:** tabelas `gold.*` da arquitetura, worker que escuta `trip_pending` e o pacote de cálculo com a lista de presets de combustível (a API2 importa a lista e não a duplica).
- **O que vem da [API1](API1-worker-gold.md):**
  - O worker preenche `boot_generation` quando ela está `NULL`, assim que o boot aparece.
  - As colunas `jsonb` de `gold.trip_series` guardam `series`, `series_lentas`, `rota` e as três distribuições no formato da spec 02. A API só as repassa e acrescenta `series.passo_ms` e `series.exposicao_passo_ms`, que ficam em colunas próprias.
  - As tabelas de linhas têm ordem: `part_no`, `event_no`, `stop_no`, `gear` e `shift_no`. `trip_speed_band` é lida por `from_kmh`.

## Endpoints

Erros usam `problem` (`application/problem+json`, campo `codigo`). Códigos novos: `entrada_invalida`, `motorista_repetido`, `viagem_desconhecida`, `analise_indisponivel`. Reaproveitados: `dongle_desconhecido`, `corpo_malformado`, `corpo_grande`, `tipo_conteudo`, `limite_taxa`, `banco_ocupado`, `erro_interno`.

### `POST /v1/viagens`

Corpo `application/json` de até 4 KiB (`limitBody(4096)`; acima, `413 corpo_grande`). Outro `Content-Type` dá `415 tipo_conteudo`. JSON inválido, tipo errado ou campo desconhecido dá `400 corpo_malformado`.

| Campo | Regra |
|---|---|
| `motorista.nome` | Obrigatório. Sem espaços nas pontas, de 1 a 60 caracteres (runes), sem caracteres de controle |
| `categoria_viagem` | Obrigatório. Sem espaços nas pontas, de 1 a 40 runes, só letras, dígitos, espaço, `_` e `-`. Gravada em minúsculas |
| `combustivel` | Obrigatório. Um dos seis presets da spec 02. `personalizado` é rejeitado |
| `entrada.tipo` | `boot` ou `intervalo`. Campos do outro tipo presentes dão `entrada_invalida` |
| `boot` | `dongle_id` inteiro ≥ 1 (id numérico do banco, não o DevId das rotas de dongle), `boot_id` de 1 a 65535, `boot_generation` opcional de 0 a 65535 |
| `intervalo` | `dongle_id`, `t0`, `t1` em RFC 3339 com fuso (`Z` ou `±hh:mm`; data sem fuso é rejeitada). `t1 > t0` e `t1 − t0 ≤ 7 dias` |

Nome normalizado (`driver_name_normalized`): `strings.TrimSpace` e minúsculas, feito em Go no `domain`. `driver_name` guarda o nome sem espaços nas pontas e com a caixa original.

Fluxo do caso de uso, em uma só `InTransaction`:

1. `States.Read(dongle_id)`. Sem linha em `dongle_state`: `404 dongle_desconhecido`.
2. `target_seq = head_seq`. Dongle sem handshake tem `head_seq = 0` (coluna `NOT NULL DEFAULT 0`) e a viagem é aceita assim.
3. Em `boot` sem `boot_generation`: `Boots.LatestGeneration(dongle_id, boot_id)`. Achou: grava a geração. Não achou: grava `NULL`, e o worker resolve quando o boot chegar.
4. `Trips.Insert` com `status = 'pending'` e os campos de cálculo nulos.
5. `Trips.Notify(trip_id)`, da API1: `SELECT pg_notify('trip_pending', trip_id::text)`. O `NOTIFY` só sai no commit.

| Resposta | Caso |
|---|---|
| `202`, `Location: /v1/viagens/{id}` e `{ "viagem_id": "<uuid>", "status": "pendente" }` | Criada |
| `400 entrada_invalida` | Qualquer regra da tabela; `detail` nomeia o campo |
| `404 dongle_desconhecido` | Dongle sem `dongle_state` |
| `409 motorista_repetido` | SQLSTATE 23505 em `trip_driver_name_unique`, traduzido no adaptador para `trip.ErrDriverRepeated` |
| `503 banco_ocupado` | `port.ErrDatabaseBusy` |

### `GET /v1/viagens/{id}`

`200` com o corpo abaixo. `{id}` que não seja UUID, que não exista ou com `excluded_at` preenchido dá `404 viagem_desconhecida`.

```json
{ "viagem_id": "…", "status": "pronta", "provisoria": false,
  "motorista": { "nome": "João" }, "categoria_viagem": "feira", "combustivel": "gasolina_e30",
  "entrada": { "tipo": "boot", "dongle_id": 1, "boot_id": 8, "boot_generation": 0 },
  "criada_em": "2026-10-28T17:03:00.123Z", "calculada_em": null }
```

- `status` traduz `pending`, `waiting_data`, `ready`, `failed` para `pendente`, `aguardando_dados`, `pronta`, `falhou`, em mapa no pacote `http` (como `wireReasons`).
- `entrada` de `intervalo` traz `t0` e `t1` em UTC (`Z`): o banco guarda `timestamptz`, e o instante é o mesmo.
- `boot_generation` sai `null` até o worker resolvê-la.
- Datas em RFC 3339 UTC com milissegundos. `calculada_em` é `null` até o primeiro cálculo.
- `Cache-Control: no-store`.

### `GET /v1/viagens/{id}/analise`

- **`200`:** JSON da spec 02, com `motorista` (`{ "id": "<viagem_id>", "nome": "…" }`) e `categoria_viagem` dentro de `viagem`. Vale para viagem `pronta`, inclusive provisória e vazia; o cliente lê `provisoria` no status.
- **`409 analise_indisponivel`:** viagem `pendente`, `aguardando_dados` ou `falhou`. O corpo traz `viagem_status` com o nome da API (o `status` do `problem` já é o código HTTP).
- **`404 viagem_desconhecida`:** como no status.
- Parâmetros de consulta são ignorados. `Cache-Control: no-store`.

**Montagem sem recalcular.** O adaptador lê todas as tabelas da viagem numa transação `REPEATABLE READ` somente leitura, para não misturar linhas de antes e depois de um recálculo. O `Transactor` é `READ COMMITTED`, então `Trips.Analysis` abre a sua.

| Chave do JSON | Origem |
|---|---|
| `versao_calculo` | `gold.trip.calculation_version` |
| `viagem` (entrada, `inicio_t_ms`, `fim_t_ms`, `inicio_em`, `fim_em`, `relogio`) | `gold.trip` e `gold.trip_summary` |
| `viagem.partes` | `gold.trip_part` |
| `parametros`, `avisos` | `gold.trip_summary` (jsonb, repassado) |
| `resumo`, `condutor`, `motor`, `qualidade`, `marchas.marcha_lenta`, `marchas.subida_rpm`, `marchas.reducao_rpm`, `marchas.ponto_morto_embreagem_s` | colunas de `gold.trip_summary` |
| `faixas_velocidade`, `eventos`, `paradas` | `gold.trip_speed_band`, `trip_event`, `trip_stop` |
| `marchas.patamares`, `marchas.trocas` | `gold.trip_gear`, `trip_gear_shift` |
| `series`, `series_lentas`, `rota`, `distribuicoes` | `gold.trip_series` (`jsonb` repassado; `passo_ms` e `exposicao_passo_ms` vêm de `step_ms` e `exposure_step_ms`) |

### Serialização

- **Sem arredondar:** `float64` do banco (`double precision`) vai ao JSON com `encoding/json`, que escreve o menor texto que reconstrói o valor.
- **Ausência é `null`:** campos numéricos são ponteiros sem `omitempty`, então a chave sempre existe. `NaN` e infinito viram `null` no adaptador, porque `encoding/json` falha neles.
- **Listas nunca são `null`:** `avisos`, `partes`, `faixas_velocidade`, `paradas`, `eventos`, `trocas` e `patamares` saem `[]` quando vazias.
- **Sem GPS:** `rota` é `null`, e `lat`/`lon` de `paradas` e `eventos` são `null`. A API repassa o que o worker gravou.
- **Sem hora absoluta:** `inicio_em`, `fim_em` e `relogio` são `null`.
- **Unidades** ficam nos nomes dos campos (spec 02). A API não converte.
- **Tamanho:** séries limitadas a 5000 pontos e rota sem redução dão de algumas centenas de KB a cerca de 1 MB, dentro do `WriteTimeout` de 30 s.
- **Compressão:** o Caddy já aplica `encode zstd gzip` no site da API (`deploy/caddy/Caddyfile`). A API não comprime. O `make e2e` usa `compose.test.yaml`, sem Caddy, e trafega sem compressão.

## Middlewares

| Rota | Cadeia |
|---|---|
| `POST /v1/viagens` | `recoverPanics`, `instrument("trip_create")`, `limitIP`, `limitBody(4096)` |
| `GET /v1/viagens/{id}` | `recoverPanics`, `instrument("trip_status")`, `limitIP` |
| `GET /v1/viagens/{id}/analise` | `recoverPanics`, `instrument("trip_analysis")`, `limitIP` |

- **Sem `limitDevID`:** a chave dele é o DevId, que não existe aqui.
- **Sem `limitConcurrency`:** ele usa `Deps.Semaphore`, o semáforo da ingestão (`INGESTAO_MAX_SIMULTANEAS`). Compartilhá-lo faria o polling de 1 s disputar vagas com os blocos do app. A carga é pequena e o pool do banco já limita.
- **`limitIP` com escopo próprio:** hoje ele usa o escopo `ip` e `LIMITE_IP_MIN` (120/min). O Next.js é um só IP e consulta o status a cada 1 s por visitante, então 120/min estouraria com dois visitantes. Cria-se `limitIPScope(escopo, limite)`, e o `limitIP` atual passa a chamá-lo. Nova variável `LIMITE_VIAGEM_IP_MIN` (padrão 600), escopo `viagem_ip`. Sem `REDIS_ENDERECO` o contador é por réplica (`API_REPLICAS`, padrão 2, `round_robin` no Caddy), então o limite efetivo é até N vezes maior. Aceito.
- **CORS:** não é necessário. O navegador só fala com o Next.js (spec 04, "Frontend"). Nenhum cabeçalho `Access-Control-*` é enviado.

## Modo sem autenticação

- Nova variável `ANALISE_SEM_AUTH` (padrão falso) em `config.Config` (`AnalysisNoAuth`), lida como `PERMITIR_PERFIL_NONE`.
- Falsa: `Router()` não registra as três rotas, e o `ServeMux` responde 404.
- `AMBIENTE=producao` com `ANALISE_SEM_AUTH=true`: `LoadFrom` acrescenta o erro `ANALISE_SEM_AUTH é proibido em AMBIENTE=producao`. Consequência: até a API3, a feira roda em `dev` ou `prodlike`.
- No serviço `api`: `compose.yaml` repassa `${ANALISE_SEM_AUTH:-false}`, e `compose.test.yaml` a define como `true`. Nunca aparece em `compose.prod.yaml`.
- A mudança de semântica na API3 está na spec 04. Esta entrega não prepara nada para ela.

## Código

| Camada | Conteúdo |
|---|---|
| `domain/trip` | Entrada, normalização de nome e categoria, `Status`, `ErrDriverRepeated`, `ErrNotFound`, `ErrNotReady` |
| `port` | A porta `Trips` da API1 ganha `Insert`, `Find` e `Analysis`; `Boots.LatestGeneration` |
| `usecase/trip` | `create_trip.go`, `get_trip.go`, `get_analysis.go`, um `Run` por arquivo |
| `adapter/postgres` | `trips.go`, `queries/trip.sql` (sqlc), `LatestGeneration` em `boots`, tradução do 23505 |
| `adapter/memory` | `Trips` em memória para os testes |
| `http` | Um arquivo por rota; interfaces em `usecases.go`; `Deps` ganha `CreateTrip`, `GetTrip`, `GetAnalysis`; `d.err` mapeia os erros novos; `problemWith` acrescenta campos ao corpo |
| `config`, `cmd/tccelta/api.go` | `AnalysisNoAuth`, `Limits.TripIPMin`; composição |

- `States.Read` do adaptador `postgres` passa a devolver `dongle.ErrUnknown` em `pgx.ErrNoRows` (hoje devolve o erro cru).
- **Permissões do `tccelta_api`:** já vêm da `00009` da API1: `USAGE` no schema `gold`, `SELECT, INSERT` em `gold.trip` e `SELECT` nas demais. Não há `UPDATE` nem `DELETE`.
- `http` e `usecase` não importam `pgx` nem `dbgen`.

## Decisões novas

| Decisão | Motivo |
|---|---|
| `{id}` que não é UUID dá `404 viagem_desconhecida` | Uma resposta só para "não existe" e "malformado"; o navegador não monta ids |
| `409 analise_indisponivel` leva `viagem_status`, não `status` | `status` do `problem` é o código HTTP |
| Nome até 60 runes, categoria até 40, intervalo até 7 dias, corpo até 4 KiB | Limites não cobertos pela arquitetura; ajustar se a feira exigir |
| Categoria gravada em minúsculas | O índice único usa `trip_category` sem normalização; "Feira" não deve abrir outro grupo |
| Boot ainda ausente: `boot_generation = NULL` | O worker resolve depois; a viagem sai vazia e provisória se nada chegar em 20 s |
| Sem idempotência no `POST` | Repetir após timeout dá `409 motorista_repetido`; o Next.js mostra o erro ao operador |

## Testes

- **`http` (`api_test.go`):** fakes dos casos de uso. Cobre validação, mapeamento de erros, UUID inválido, `Content-Type`, `limitBody`, rotas ausentes sem `ANALISE_SEM_AUTH`, escopo do `limitIP` e `X-Request-Id`.
- **Caso de uso, com `memory`:** `head_seq = 0`, geração resolvida, geração `NULL`, nome repetido com caixa e espaços diferentes, nome liberado após `failed`.
- **`config_test.go`:** `ANALISE_SEM_AUTH=true` falha em `producao` e passa em `dev` e `prodlike`.
- **Integração (`-tags integration`):**
  - o `NOTIFY` chega a um `LISTEN trip_pending` após o commit, com o `trip_id` como carga, e não chega após rollback;
  - dois `POST` concorrentes com o mesmo nome: um `202`, um `409`;
  - `tccelta_api` não faz `UPDATE` nem `DELETE` em `gold.trip`;
  - `Analysis` com uma transação de recálculo aberta devolve o resultado anterior inteiro.
- **E2E:** novo cenário `trip-http` em `internal/tools/simulator`, na lista do `make e2e`, com worker e `ANALISE_SEM_AUTH=true` no `compose.test.yaml`. O cenário `trip` da API1 insere direto no banco; este passa pela API.
  1. Envia blocos de um boot, faz `POST` e consulta o status até `pronta` (limite de 40 s). Confere no `GET .../analise` campos conhecidos do simulador e `viagem.motorista`.
  2. `POST` de um boot ainda não enviado, depois envio dos blocos: a viagem fica `pronta`, com `boot_generation` resolvida e dados.
  3. `POST` de um boot que nunca chega: a viagem fica `pronta`, provisória, com `sem_leituras`, em cerca de 20 s.

## Critérios de aceite

1. Sem `ANALISE_SEM_AUTH`, as três rotas respondem 404; com `true`, respondem conforme esta spec.
2. `AMBIENTE=producao` com `ANALISE_SEM_AUTH=true` impede a API de subir.
3. `POST` válido de `boot` devolve `202`, `Location` e `status = pendente`; a linha de `gold.trip` tem `status = 'pending'` e `target_seq` igual ao `head_seq` lido na mesma transação.
4. Um `LISTEN trip_pending` recebe o `viagem_id` após um `POST` válido e nada após um `POST` que falha.
5. `boot` sem `boot_generation` grava a maior geração existente do `boot_id`; sem boot nenhum, grava `NULL` e responde `202`.
6. `dongle_id` sem `dongle_state` devolve `404 dongle_desconhecido`; dongle sem handshake é aceito com `target_seq = 0`.
7. `"  JOÃO "` e `"joão"` na mesma categoria: o segundo `POST` devolve `409 motorista_repetido`; em outra categoria, `202`. Com a primeira viagem em `failed`, o nome é aceito.
8. Cada violação da tabela do `POST` devolve `400 entrada_invalida` com o campo em `detail`: `personalizado`, `t1 ≤ t0`, `t0` sem fuso, intervalo de 8 dias, duas entradas, nenhuma, nome de 61 runes.
9. Corpo de 4097 bytes devolve `413`; `Content-Type: text/plain` devolve `415`; campo desconhecido devolve `400 corpo_malformado`.
10. `GET /v1/viagens/abc` e um UUID inexistente devolvem `404 viagem_desconhecida`.
11. O status mostra `pendente`, `aguardando_dados`, `pronta` e `falhou` para os quatro valores do banco, com `provisoria` e `calculada_em` do banco.
12. `GET .../analise` de viagem não `pronta` devolve `409 analise_indisponivel` com `viagem_status`.
13. A análise de viagem `pronta` tem as chaves da spec 02 mais `viagem.motorista` e `viagem.categoria_viagem`. Um valor `0.30000000000000004` gravado no banco sai idêntico.
14. Viagem vazia: `resumo.distancia_km` é `null`, as listas são `[]`, `rota` é `null`, e `avisos` traz `sem_leituras`.
15. Sem GPS: `rota`, `lat` e `lon` são `null`. Sem conversão de hora: `relogio`, `inicio_em` e `fim_em` são `null`.
16. `tccelta_http_requests_total` tem as rotas `trip_create`, `trip_status` e `trip_analysis`, e as respostas trazem `X-Request-Id`.
17. Com `LIMITE_VIAGEM_IP_MIN=600`, 130 `GET` por minuto de um IP não recebem `429`; acima do limite, recebem `429 limite_taxa`.
18. `tccelta_api` não executa `UPDATE` nem `DELETE` em `gold.trip` e não escreve nas outras tabelas da gold.
19. `make lint` passa, sem `pgx` ou `dbgen` em `http` e `usecase`.
20. O cenário `trip-http` do `make e2e` passa nos três casos de "Testes".
