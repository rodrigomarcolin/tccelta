# Métricas expostas pela API

A API e o worker expõem métricas no formato Prometheus. Este documento descreve cada série da família `tccelta_*`,
os rótulos e os valores que o código produz. As séries foram lidas de `internal/observability/metrics/metrics.go`
e dos pontos do código que as incrementam; **nada aqui foi conferido contra uma execução**: o Prometheus e a coleta
real nunca rodaram neste projeto.

## Onde ler

| Processo | Endereço (dentro da rede do compose) | Variável | Séries |
|---|---|---|---|
| API (cada réplica) | `http://api:9090/metricas` | `METRICAS_ENDERECO` (padrão `:9090`) | `tccelta_*` |
| Worker | `http://worker:9091/metricas` | `METRICAS_ENDERECO` (padrão `:9091`) | `tccelta_*` |
| KMS | `http://kms:9300/metricas` | `KMS_METRICAS_ENDERECO` | `tccelta_kmssvc_*` (veja o fim) |

As portas não são publicadas no host. Para consultar, use um contêiner na rede `kms` (que a API integra):

```bash
docker run --rm --network tccelta_kms curlimages/curl:8.10.1 -sS http://api:9090/metricas | grep '^tccelta_'
```

O perfil `observabilidade` (`docker compose --profile observabilidade up -d`) sobe o Prometheus em
`127.0.0.1:9099` e o Grafana em `127.0.0.1:3000`. O `deploy/observability/prometheus.yml` coleta os três alvos
(`api:9090`, `worker:9091`, `kms:9300`) a cada 15 s.

## Como ler as séries

- Os processos usam um registro próprio. Por isso **não há** séries `go_*` nem `process_*` (memória, goroutines,
  CPU): só as `tccelta_*` abaixo.
- Nenhuma série tem texto de ajuda (`# HELP`) no `/metricas`; este documento é a descrição.
- Contadores com rótulos (`_total{...}`) só aparecem depois do primeiro evento daquele valor. Uma série ausente
  quer dizer "ainda não aconteceu", não "zero".
- Histogramas geram `_bucket{le=...}`, `_sum` e `_count`. Os limites dos baldes estão na tabela.
- Cada réplica da API tem o seu próprio contador. Some (`sum`) entre as réplicas ao agregar.
- Os rótulos de `route` usam o nome curto da rota, não o caminho.

## HTTP (API)

| Série | Tipo | Rótulos | O que mede |
|---|---|---|---|
| `tccelta_http_requests_total` | contador | `route`, `status` | Requisições respondidas, por rota e código HTTP |
| `tccelta_http_duration_seconds` | histograma | `route` | Tempo de atendimento. Baldes: 5 ms a 10 s |

Valores de `route`: `challenge` (`POST /v1/dongles/{id}/desafio`), `response` (`POST …/desafio/resposta`), `blocks`
(`POST /v1/dongles/blocos`) e `confirmation` (`GET …/confirmacao`). As rotas `/saude/viva` e `/saude/pronta` **não**
são medidas.

| Série | Tipo | Rótulos | O que mede |
|---|---|---|---|
| `tccelta_rejections_total` | contador | `reason` | Requisições recusadas antes de processar |

Valores de `reason` encontrados no código: `rate_limit` (limite por IP ou por DevId estourado), `overload`
(limite de concorrência), `database_busy` e `kms` (dependência indisponível, resposta 503). Alerte em crescimento
sustentado de `kms` e `database_busy`.

## Handshake

| Série | Tipo | Rótulos | O que mede |
|---|---|---|---|
| `tccelta_handshakes_total` | contador | `result` | Desfecho de cada etapa do handshake |
| `tccelta_anchor_rtt_seconds` | histograma | — | Tempo de ida e volta entre o desafio e a resposta, usado como âncora de tempo. Baldes: 50 ms a 10 s |

Valores de `result`:

| Valor | Significado |
|---|---|
| `challenge_issued` | O servidor emitiu um desafio |
| `invalid_challenge` | A resposta citou um desafio inexistente, expirado ou já usado |
| `stale_c_s` | A resposta veio com contador de sessão antigo |
| `invalid_tag` | A tag da resposta não confere com a chave do dongle |
| `inconsistent` | O HEAD informado é menor que o já visto (HTTP 422 `resposta_inconsistente`) |
| `accepted` | Sessão aberta, com âncora de tempo |
| `accepted_no_anchor` | Sessão aberta, mas sem âncora de tempo válida |

## Ingestão de blocos

| Série | Tipo | Rótulos | O que mede |
|---|---|---|---|
| `tccelta_batches_total` | contador | `status` | Lotes recebidos, por desfecho |
| `tccelta_blocks_total` | contador | `result` | Blocos dos lotes, por desfecho |
| `tccelta_ingestion_tx_duration_seconds` | histograma | — | Duração da transação de gravação de um lote. Baldes: 5 ms a 5 s |
| `tccelta_inserted_rows_total` | contador | `table` | Linhas gravadas nas tabelas de dados |
| `tccelta_gaps_total` | contador | `cause` | Lacunas registradas no log do dongle |

- `tccelta_batches_total{status}`: `completo`, `sem_progresso` (o lote foi recusado já no primeiro bloco por tag
  inválida, sem avançar o ACK) e `interrompido` (parou no meio; o resto fica para o próximo lote).
- `tccelta_blocks_total{result}`: `accepted`, `duplicate`, `discarded_after_interruption` e os motivos de rejeição
  `invalid_tag`, `jump`, `invalid_marker`, `unknown_pid` e `malformed_record`. Rejeições também vão
  para a tabela `rejected_block`. Crescimento de `invalid_tag` é o sinal mais importante: chave errada ou bloco
  adulterado.
- `tccelta_inserted_rows_total{table}`: `reading` e `position`.
- `tccelta_gaps_total{cause}`: `BOOT` (reinício do dongle), `LOSS` (bloco perdido ou corrompido) e `OVERFLOW`
  (a flash do dongle encheu e apagou blocos antigos). É emitida tanto na ingestão quanto no handshake.

## KMS visto pela API

| Série | Tipo | Rótulos | O que mede |
|---|---|---|---|
| `tccelta_kms_calls_total` | contador | `operation`, `result`, `phase` | Chamadas da API ao serviço KMS |
| `tccelta_kms_duration_seconds` | histograma | `operation` | Duração de cada chamada. Baldes: 1 ms a 2 s |
| `tccelta_kms_breaker_open` | medidor | — | `1` com o disjuntor aberto (a API para de chamar o KMS por um tempo), `0` fechado |

- `operation`: `hmac_auth` (HMAC do desafio), `verify_rec` (verificação das tags dos blocos) e `sign_jwt`
  (assinatura do JWT).
- `result`: `ok` ou `erro`.
- `phase`: `pre` (chamada fora da transação do banco) ou `tx` (dentro dela). Chamadas lentas na `phase` `tx` seguram a
  conexão do banco.

Com o KMS fora do ar, `tccelta_kms_calls_total{result="erro"}` e `tccelta_rejections_total{reason="kms"}`
crescem e `tccelta_kms_breaker_open` vai a `1` (cenário do P1-25 do roteiro).

## Limites e banco

| Série | Tipo | Rótulos | O que mede |
|---|---|---|---|
| `tccelta_limiter_redis_failures_total` | contador | — | Vezes em que o Redis falhou e o limitador caiu para o limite local, por réplica |
| `tccelta_pool_connections_total` | medidor | — | Conexões abertas no pool do Postgres |
| `tccelta_pool_connections_in_use` | medidor | — | Conexões em uso agora |
| `tccelta_pool_connections_idle` | medidor | — | Conexões livres |

A API e o worker registram o pool, cada um com o seu. `connections_in_use` perto de `connections_total` por muito tempo indica falta de
conexões e costuma vir junto com `rejections_total{reason="database_busy"}`.

## Worker

O worker recalcula a reta de tempo de cada boot (do relógio do dongle para o relógio real) e expira desafios.

| Série | Tipo | Rótulos | O que mede |
|---|---|---|---|
| `tccelta_worker_processed_boots_total` | contador | `source` | Boots recalculados, por origem da estimativa |
| `tccelta_worker_duration_seconds` | histograma | — | Duração de cada ciclo de processamento. Baldes: 1 ms a 5 s |
| `tccelta_worker_time_queue` | medidor | — | Boots aguardando recálculo de tempo |
| `tccelta_worker_alerts_total` | contador | `type` | Alertas gerados pela estimativa de tempo |
| `tccelta_worker_anchor_groups_written_total` | contador | — | Grupos de âncoras gravados |
| `tccelta_worker_expired_challenges_total` | contador | — | Desafios vencidos removidos |
| `tccelta_worker_listen_reconnections_total` | contador | — | Reconexões da escuta de notificações do Postgres |

- `source`: `anchors` (estimada com as âncoras do próprio boot), `neighbors` (deduzida dos boots vizinhos) e
  `no_estimate` (não foi possível estimar).
- `type` de alerta: `empty_intersection` (as âncoras do boot não são compatíveis entre si) e `excessive_drift`
  (a deriva do relógio do dongle passou do limite). Cada alerta novo também é gravado na tabela `alert`.
- `tccelta_worker_time_queue` que só cresce indica que o worker não acompanha a ingestão.

## Reprocessamento

| Série | Tipo | O que mede |
|---|---|---|
| `tccelta_reprocessing_rows_total` | contador | Linhas regeneradas pelo comando de reprocessamento |

Aparece no processo que roda o reprocessamento (a CLI `tccelta`), não na API em execução.

## Métricas do KMS (`tccelta_kmssvc_*`)

O serviço KMS tem as suas próprias séries, em `kms:9300/metricas`, separadas das da API. Os nomes encontrados no
código são: `tccelta_kmssvc_http_requests_total`, `tccelta_kmssvc_http_duration_seconds`,
`tccelta_kmssvc_vault_calls_total`, `tccelta_kmssvc_vault_duration_seconds`,
`tccelta_kmssvc_vault_breaker_open`, `tccelta_kmssvc_rejections_total`, `tccelta_kmssvc_jwt_rotations_total` e
`tccelta_kmssvc_jwt_last_check_ok`. Os rótulos e a semântica delas ficam para o `docs/kms.md`; não os revisei
aqui.

## Consultas úteis

Exemplos em PromQL. Não foram executados.

```promql
# Taxa de erro 5xx por rota, janela de 5 min
sum by (route) (rate(tccelta_http_requests_total{status=~"5.."}[5m]))
  / sum by (route) (rate(tccelta_http_requests_total[5m]))

# Latência p95 da ingestão de blocos
histogram_quantile(0.95, sum by (le) (rate(tccelta_http_duration_seconds_bucket{route="blocks"}[5m])))

# Blocos recusados por motivo
sum by (result) (rate(tccelta_blocks_total{result!~"accepted|duplicate"}[5m]))

# KMS indisponível
max(tccelta_kms_breaker_open) == 1

# Pool do banco saturado
max(tccelta_pool_connections_in_use / tccelta_pool_connections_total) > 0.9
```
