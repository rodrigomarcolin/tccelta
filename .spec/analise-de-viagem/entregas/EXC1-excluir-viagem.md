# EXC1 — Excluir viagem

> **Status: especificação de algo a desenvolver.** Esta entrega deixa o operador excluir uma viagem pela lista de viagens e pela tela da análise. As decisões de arquitetura estão em [`04-arquitetura.md`](../04-arquitetura.md); a listagem e as rotas que esta entrega estende estão na [API2](API2-endpoints-viagem.md) e na [INT1](INT1-front-integrado.md). Aqui entra só o que é novo.

## Objetivo

`DELETE /v1/viagens/{id}` no `backend/`, preenchendo `gold.trip.excluded_at`, e a ação "Excluir" no `frontend/`, com diálogo de confirmação. A viagem excluída some da lista, da análise e do worker, e libera o nome do motorista e a entrada (boot ou intervalo) para uma nova viagem.

## Escopo e fora de escopo

| Dentro | Fora |
|---|---|
| `DELETE /v1/viagens/{id}` com o Bearer de client credentials | Restaurar uma viagem excluída (só por SQL) |
| Exclusão em qualquer status, inclusive em processamento | Registrar quem excluiu (`excluded_by`) |
| Worker ignorando viagens excluídas | Apagar as linhas de fato (exclusão física, expurgo) |
| Botão no card da lista e na tela da análise, com diálogo | Exclusão em lote, lista de excluídas |
| Fonte mock seguindo a mesma regra | Gatilho do ranking (entra com o ranking, ver "Ranking") |

## Decisões

| Decisão | Motivo |
|---|---|
| Exclusão lógica: `UPDATE ... SET excluded_at = now()`, sem `DELETE` | A coluna e os índices parciais já existem; as linhas filhas ficam para auditoria por SQL e, no futuro, para `gold.ranking_*`, que referencia `gold.trip` |
| Confirmação por diálogo; sem desfazer na tela | Restaurar pode colidir com o nome ou a entrada já reusados (409); fica para depois, se fizer falta |
| Sem registro de autor | Escolha do usuário; só `excluded_at` |
| Qualquer status pode ser excluído | Desfaz uma viagem criada errada sem esperar o limite de 20 s; o worker passa a ignorá-la |
| Repetir a exclusão dá `404 viagem_desconhecida` | Mesma regra de `GET /v1/viagens/{id}` para viagem excluída; o front trata `404` como "já não existe" e segue como sucesso |
| `204 No Content`, sem corpo | Não há o que devolver; a lista e a análise já não mostram a viagem |

## Backend

### `DELETE /v1/viagens/{id}`

Mesmos middlewares das outras rotas de viagem: `tripAuth(vip)` (limite `viagem_ip` e `authenticateAnalise`). Rota de métrica `trip_delete`. Sem corpo; um corpo enviado é ignorado.

| Resposta | Caso |
|---|---|
| `204` | Excluída agora |
| `404 viagem_desconhecida` | `{id}` fora do UUID canônico (`tripIDFromPath`), viagem que não existe ou já excluída |
| `401 token_ausente`, `token_expirado`, `token_invalido` | Como nas outras rotas de viagem |
| `429 limite_taxa` | Limite `viagem_ip` |
| `503 banco_ocupado` com `Retry-After` | `lock_timeout` de 3 s: o worker está com a viagem travada calculando (ver "Concorrência") |

### Banco

**Migration `00012_trip_exclude.sql`.**

```sql
-- +goose Up
GRANT UPDATE (excluded_at) ON gold.trip TO tccelta_api;
-- +goose Down
REVOKE UPDATE (excluded_at) ON gold.trip FROM tccelta_api;
```

O `GRANT` é por coluna: o `tccelta_api` continua sem poder mudar `status`, nome ou entrada. Atualizar a tabela de permissões da arquitetura.

**Query nova em `queries/trip.sql`:**

```sql
-- name: ExcludeTrip :one
UPDATE gold.trip SET excluded_at = now()
WHERE trip_id = @trip_id AND excluded_at IS NULL
RETURNING trip_id;
```

Sem linha (`pgx.ErrNoRows`) vira `trip.ErrNotFound`.

**Worker passa a ignorar excluídas.** Acrescentar `AND excluded_at IS NULL` em:

| Query | Por quê |
|---|---|
| `ListWaitingTrips` | Senão o ticker reavalia a viagem a cada volta |
| `TakeTrip` | Viagem excluída em `pending`/`waiting_data` não é calculada; `found=false` já é tratado |
| `TakeRecalcTrip` | Não recalcula excluída marcada antes com `recalc_pending` |
| `ListProvisionalTrips` | Não reavalia provisória excluída |
| `MarkTripsRecalc` (por faixa) | O `worker reprocess` não marca excluídas |
| `CountTripQueue` | A métrica da fila não conta excluídas |

`MarkTripWaiting`, `ResolveTripGeneration`, `ClearTripProvisional` e `FailTrip` ficam como estão: rodam com a viagem já obtida e não causam efeito visível numa excluída. Os índices `trip_driver_name_unique`, `trip_boot_unique`, `trip_interval_unique` e `trip_list` já filtram `excluded_at IS NULL`, então o nome e a entrada ficam livres sem mudança.

### Concorrência

O cálculo segura `FOR UPDATE` na linha da viagem até o commit. O `UPDATE` da exclusão espera essa trava:

| Situação | O que acontece |
|---|---|
| Exclusão antes do `Take` | O `Take` não acha a viagem e o worker segue para a próxima |
| Exclusão durante o cálculo, que termina em menos de 3 s | O `UPDATE` espera, o cálculo grava `ready`, e a exclusão vale em seguida |
| Cálculo segura a trava por mais de 3 s | `lock_timeout` → `port.ErrDatabaseBusy` → `503 banco_ocupado`; o front mostra o erro no diálogo e o operador tenta de novo |
| Cálculo falha depois da exclusão (o `failTx` roda em outra transação) | `FailTrip` marca `failed` numa viagem já excluída; ela segue fora de tudo |

Por isso o caso de uso roda em `InTransaction` com `database.DefaultOptions` (`lock_timeout` 3 s), não direto no pool como o `GetTrip`.

### Código

| Arquivo | Mudança |
|---|---|
| `internal/port/trip.go` | `Trips.Exclude(ctx, id) error`: `trip.ErrNotFound` se não existe ou já foi excluída |
| `internal/adapter/postgres/trips.go` | `Exclude` com `ExcludeTrip`; `sqlc generate` para `dbgen` |
| `internal/adapter/memory/trips.go` | `Exclude`, e os filtros de excluída nos métodos do worker listados acima |
| `internal/usecase/trip/delete_trip.go` | `DeleteTrip.Run(ctx, id) error` dentro de `InTransaction` |
| `internal/http/delete_trip.go` | Handler: `tripIDFromPath`, chama o caso de uso, `204` ou `d.err` |
| `internal/http/usecases.go`, `router.go` | Interface `DeleteTrip`, campo em `Deps`, `mux.Handle("DELETE /v1/viagens/{id}", base("trip_delete", d.deleteTrip, tripAuth(vip)...))` |
| `cmd/tccelta/api.go` | `DeleteTrip: tripUC.NewDeleteTrip(...)` |
| `internal/tools/simulator/scenario_trip_http.go` | Passo novo no cenário de viagem: exclui, confere `404` no status e recria a mesma entrada com o mesmo nome (`202`) |
| `backend/docs/cenarios-simulador.md` | Descrever o passo novo |

### Ranking

Quando o ranking existir, `Exclude` marca `gold.ranking_state.pending` como último comando da transação, como já prevê a arquitetura ("Gatilhos"). Nada a fazer nesta entrega.

## Frontend

### Camada de dados

| Arquivo | Mudança |
|---|---|
| `server/trips/datasource.ts` | `TripsDatasource.remove(id): Promise<ApiResponse<null>>` (`DELETE /v1/viagens/{id}`) |
| `server/trips/apiDatasource.ts` | `call` aceita `"DELETE"`; `204` volta `{ status: 204, body: null }` sem `JSON.parse` (hoje um corpo vazio vira `502 resposta_invalida`). `remove` confere o UUID como `status`. O novo token após `401` vale também aqui: a API recusa o Bearer antes da rota |
| `server/trips/mock/mockDatasource.ts` | Marca a viagem como excluída: some de `list`, `status` e `analysis` dão `404`, e o nome e a entrada ficam livres para `create` |
| `app/api/trips/[id]/route.ts` | `DELETE` com `withSession` e `validId`; repassa `204` ou o `problem` |
| `datasources/tripDatasource.ts` | `deleteTrip(id)`: resolve em `204` e em `404` (já excluída); outros erros lançam como nas demais funções |
| `use-cases/useDeleteTrip.ts` | `useMutation`. No sucesso: cancela e remove as queries `["trip", id]`, tira a viagem de todas as páginas em cache de `["trips", *]` (`setQueriesData`, com `total − 1`) e invalida `["trips"]` |

O `DELETE` cross-site não sai do navegador sem preflight CORS, que o Next.js não responde, então a sessão por cookie basta, como no `POST`.

### Diálogo de confirmação

Componente novo `components/ui/ConfirmDialog.tsx`, sobre `<dialog>` nativo com `showModal()` (foco preso, `Esc` cancela, fundo inerte).

- **Título:** "Excluir a viagem de {nome}?"; sem nome conhecido, "Excluir esta viagem?".
- **Texto:** "Ela some da lista e da análise, e o nome e o boot ficam livres para uma nova viagem. Não é possível desfazer."
- **Botões:** "Cancelar" (`secondary`, foco inicial) e "Excluir" (variante nova `danger` em `Button.tsx`, com o tom de alerta do design system).
- **Durante o pedido:** os dois botões desabilitados e "Excluindo…"; o diálogo não fecha com `Esc`.
- **Erro:** fica aberto com `Note` "Não foi possível excluir. Tente de novo." (`503`, `api_indisponivel`, rede). `401`/`403` seguem o tratamento de sessão que já existe.

### Lista (`/viagens`)

- `TripCard`: o card hoje é um `Link` inteiro, e botão dentro de link é HTML inválido. O `article` passa a `relative`, o `Link` continua cobrindo o card, e o botão de lixeira é irmão do `Link`, no canto superior direito, sempre visível (sem depender de hover, para o celular). `aria-label` "Excluir a viagem de {nome}". Ícone novo `IconTrash` em `components/icons/Icons.tsx`.
- O espaço do botão entra no padding do texto, para o nome truncado não ficar embaixo dele.
- Depois de excluir, o card sai da lista na hora (cache), o foco vai para o card seguinte (ou o anterior, se era o último) e um `role="status"` anuncia "Viagem excluída". `aria-setsize` e a nota de fim ("Todas as N viagens…") usam o total novo.

### Tela da análise (`/viagem/[id]`)

- Botão "Excluir viagem" (`secondary` com `IconTrash`, `print:hidden`) no `Header` da análise pronta e no `SimplePage` das fases `waiting`, `limit` e `failed`. Fora de `loading`, `not_found` e `error`, em que não se sabe se a viagem existe.
- No sucesso: para o polling (`cancelQueries` antes da mutação, para não piscar o `NotFoundState`) e `router.replace("/viagens")`.
- Outro operador com a mesma viagem aberta: se ela ainda está em polling, o próximo `404` já leva ao `NotFoundState`; uma análise pronta e não provisória fica na tela até recarregar. Aceito.

## Testes

**Backend**

- Handler (`trip_routes_test.go`): `204`; `404` para id inválido, inexistente e já excluído; `401` sem token; `503` com `ErrDatabaseBusy`.
- Caso de uso (`trip_test.go`): repassa `ErrNotFound` e roda em transação.
- Integração Postgres (`trips_unique_list_integration_test.go` ou arquivo novo, tag `integration`):
  - Depois de `Exclude`: `Find` e `Analysis` dão `ErrNotFound`, `List` não traz a viagem e o `total` cai.
  - Mesmo nome na mesma categoria e mesma entrada (boot e intervalo) voltam a ser aceitos por `Insert` e `FindSame`.
  - Viagem `pending` excluída: `ListWaiting` não a traz e `Take` dá `found=false`; idem `TakeRecalc`, `ListProvisional`, `MarkRecalcByRange` e `CountQueue`.
  - Com o papel `tccelta_api` (`TEST_BANCO_API_URL`): `Exclude` funciona, e `UPDATE gold.trip SET status = ...` é negado.
  - Uma transação segurando `FOR UPDATE` na viagem por mais de 3 s faz `Exclude` dar `ErrDatabaseBusy`.
- `make e2e`: o passo novo do cenário de viagem.

**Frontend**

- `apiDatasource.test.ts`: `204` sem corpo, `404`, novo token após `401`, `503` com `Retry-After`.
- `mockDatasource.test.ts` e `mockList.test.ts`: excluída some da lista, dá `404` e libera nome e entrada.
- `routes.test.ts`: `DELETE` com `401`, `403`, `404` para id inválido e `204`.
- `ConfirmDialog` (novo teste): foco inicial em "Cancelar", `Esc` cancela, botões desabilitados durante o pedido, erro mantém aberto.
- `TripCard.test.tsx`: o botão não fica dentro do link e tem o `aria-label` com o nome.
- `TripsScreen.test.tsx`: excluir tira o card, atualiza o total, move o foco e anuncia; erro deixa o card.
- `TripPage.test.tsx`: excluir na análise pronta e na espera leva a `/viagens` sem passar pelo `NotFoundState`.

## Critérios de aceite

1. Na lista, a lixeira de um card abre o diálogo; "Excluir" tira o card sem recarregar a página, e ele não volta ao rolar nem ao buscar.
2. Na análise (pronta, em espera ou com falha), "Excluir viagem" confirma e volta para `/viagens` sem mostrar "viagem desconhecida".
3. `GET /v1/viagens/{id}` e `/analise` de uma excluída dão `404 viagem_desconhecida`; um segundo `DELETE` também.
4. Depois de excluir, criar uma viagem com o mesmo nome e a mesma entrada dá `202`, não `409`.
5. Uma viagem excluída enquanto `pendente` nunca chega a `ready` e não aparece na fila do worker nem na métrica da fila.
6. O papel `tccelta_api` só consegue alterar `excluded_at` em `gold.trip`.
7. Sem sessão de operador, `DELETE /api/trips/{id}` dá `401`/`403`; sem Bearer, `DELETE /v1/viagens/{id}` dá `401`.
8. Com a fonte mock, o mesmo fluxo funciona sem a API.

## Follow-ups

- **Restaurar:** rota `POST /v1/viagens/{id}/restaurar` com `409` se o nome ou a entrada já foram reusados, e uma lista de excluídas para o operador.
- **Expurgo:** os nomes dos visitantes ficam no banco após a exclusão; se for preciso apagar de fato (LGPD), um comando do worker que remove as excluídas há mais de N dias e suas linhas filhas, depois que o ranking deixar de referenciá-las.
