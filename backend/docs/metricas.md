# Métricas expostas pela API

A API e o worker expõem métricas no formato Prometheus. Este documento descreve cada série da família `tccelta_*`,
os rótulos e os valores que o código produz. As séries foram lidas de `internal/observabilidade/metricas/metricas.go`
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
`127.0.0.1:9099` e o Grafana em `127.0.0.1:3000`. O `deploy/observabilidade/prometheus.yml` coleta os três alvos
(`api:9090`, `worker:9091`, `kms:9300`) a cada 15 s.

## Como ler as séries

- Os processos usam um registro próprio. Por isso **não há** séries `go_*` nem `process_*` (memória, goroutines,
  CPU): só as `tccelta_*` abaixo.
- Nenhuma série tem texto de ajuda (`# HELP`) no `/metricas`; este documento é a descrição.
- Contadores com rótulos (`_total{...}`) só aparecem depois do primeiro evento daquele valor. Uma série ausente
  quer dizer "ainda não aconteceu", não "zero".
- Histogramas geram `_bucket{le=...}`, `_sum` e `_count`. Os limites dos baldes estão na tabela.
- Cada réplica da API tem o seu próprio contador. Some (`sum`) entre as réplicas ao agregar.
- Os rótulos de `rota` usam o nome curto da rota, não o caminho.

## HTTP (API)

| Série | Tipo | Rótulos | O que mede |
|---|---|---|---|
| `tccelta_http_requisicoes_total` | contador | `rota`, `status` | Requisições respondidas, por rota e código HTTP |
| `tccelta_http_duracao_segundos` | histograma | `rota` | Tempo de atendimento. Baldes: 5 ms a 10 s |

Valores de `rota`: `desafio` (`POST /v1/dongles/{id}/desafio`), `resposta` (`POST …/desafio/resposta`), `blocos`
(`POST /v1/dongles/blocos`) e `confirmacao` (`GET …/confirmacao`). As rotas `/saude/viva` e `/saude/pronta` **não**
são medidas.

| Série | Tipo | Rótulos | O que mede |
|---|---|---|---|
| `tccelta_rejeicoes_total` | contador | `motivo` | Requisições recusadas antes de processar |

Valores de `motivo` encontrados no código: `limite_taxa` (limite por IP ou por DevId estourado), `sobrecarga`
(limite de concorrência), `banco_ocupado` e `kms` (dependência indisponível, resposta 503). Alerte em crescimento
sustentado de `kms` e `banco_ocupado`.

## Handshake

| Série | Tipo | Rótulos | O que mede |
|---|---|---|---|
| `tccelta_handshakes_total` | contador | `resultado` | Desfecho de cada etapa do handshake |
| `tccelta_ancora_rtt_segundos` | histograma | — | Tempo de ida e volta entre o desafio e a resposta, usado como âncora de tempo. Baldes: 50 ms a 10 s |

Valores de `resultado`:

| Valor | Significado |
|---|---|
| `desafio_emitido` | O servidor emitiu um desafio |
| `desafio_invalido` | A resposta citou um desafio inexistente, expirado ou já usado |
| `c_s_antigo` | A resposta veio com contador de sessão antigo |
| `tag_invalida` | A tag da resposta não confere com a chave do dongle |
| `inconsistente` | O HEAD informado é menor que o já visto (HTTP 422 `resposta_inconsistente`) |
| `aceito` | Sessão aberta, com âncora de tempo |
| `aceito_sem_ancora` | Sessão aberta, mas sem âncora de tempo válida |

## Ingestão de blocos

| Série | Tipo | Rótulos | O que mede |
|---|---|---|---|
| `tccelta_lotes_total` | contador | `status` | Lotes recebidos, por desfecho |
| `tccelta_blocos_total` | contador | `resultado` | Blocos dos lotes, por desfecho |
| `tccelta_ingestao_tx_duracao_segundos` | histograma | — | Duração da transação de gravação de um lote. Baldes: 5 ms a 5 s |
| `tccelta_linhas_inseridas_total` | contador | `tabela` | Linhas gravadas nas tabelas de dados |
| `tccelta_lacunas_total` | contador | `causa` | Lacunas registradas no log do dongle |

- `tccelta_lotes_total{status}`: `completo`, `sem_progresso` (o lote foi recusado já no primeiro bloco por tag
  inválida, sem avançar o ACK) e `interrompido` (parou no meio; o resto fica para o próximo lote).
- `tccelta_blocos_total{resultado}`: `aceito`, `duplicado`, `descartado_apos_interrupcao` e os motivos de rejeição
  `tag_invalida`, `salto`, `marcador_invalido`, `pid_desconhecido` e `registro_malformado`. Rejeições também vão
  para a tabela `bloco_rejeitado`. Crescimento de `tag_invalida` é o sinal mais importante: chave errada ou bloco
  adulterado.
- `tccelta_linhas_inseridas_total{tabela}`: `leitura` e `posicao`.
- `tccelta_lacunas_total{causa}`: `BOOT` (reinício do dongle), `LOSS` (bloco perdido ou corrompido) e `OVERFLOW`
  (a flash do dongle encheu e apagou blocos antigos). É emitida tanto na ingestão quanto no handshake.

## KMS visto pela API

| Série | Tipo | Rótulos | O que mede |
|---|---|---|---|
| `tccelta_kms_chamadas_total` | contador | `operacao`, `resultado`, `fase` | Chamadas da API ao serviço KMS |
| `tccelta_kms_duracao_segundos` | histograma | `operacao` | Duração de cada chamada. Baldes: 1 ms a 2 s |
| `tccelta_kms_disjuntor_aberto` | medidor | — | `1` com o disjuntor aberto (a API para de chamar o KMS por um tempo), `0` fechado |

- `operacao`: `hmac_auth` (HMAC do desafio), `verify_rec` (verificação das tags dos blocos) e `sign_jwt`
  (assinatura do JWT).
- `resultado`: `ok` ou `erro`.
- `fase`: `pre` (chamada fora da transação do banco) ou `tx` (dentro dela). Chamadas lentas na fase `tx` seguram a
  conexão do banco.

Com o KMS fora do ar, `tccelta_kms_chamadas_total{resultado="erro"}` e `tccelta_rejeicoes_total{motivo="kms"}`
crescem e `tccelta_kms_disjuntor_aberto` vai a `1` (cenário do P1-25 do roteiro).

## Limites e banco

| Série | Tipo | Rótulos | O que mede |
|---|---|---|---|
| `tccelta_limite_redis_falhas_total` | contador | — | Vezes em que o Redis falhou e o limitador caiu para o limite local, por réplica |
| `tccelta_pool_conexoes_total` | medidor | — | Conexões abertas no pool do Postgres |
| `tccelta_pool_conexoes_em_uso` | medidor | — | Conexões em uso agora |
| `tccelta_pool_conexoes_ociosas` | medidor | — | Conexões livres |

A API e o worker registram o pool, cada um com o seu. `em_uso` perto de `total` por muito tempo indica falta de
conexões e costuma vir junto com `rejeicoes_total{motivo="banco_ocupado"}`.

## Worker

O worker recalcula a reta de tempo de cada boot (do relógio do dongle para o relógio real) e expira desafios.

| Série | Tipo | Rótulos | O que mede |
|---|---|---|---|
| `tccelta_worker_boots_processados_total` | contador | `origem` | Boots recalculados, por origem da estimativa |
| `tccelta_worker_duracao_segundos` | histograma | — | Duração de cada ciclo de processamento. Baldes: 1 ms a 5 s |
| `tccelta_worker_fila_tempo` | medidor | — | Boots aguardando recálculo de tempo |
| `tccelta_worker_alertas_total` | contador | `tipo` | Alertas gerados pela estimativa de tempo |
| `tccelta_worker_grupos_gravados_total` | contador | — | Grupos de âncoras gravados |
| `tccelta_worker_desafios_expirados_total` | contador | — | Desafios vencidos removidos |
| `tccelta_worker_escuta_reconexoes_total` | contador | — | Reconexões da escuta de notificações do Postgres |

- `origem`: `ancoras` (estimada com as âncoras do próprio boot), `vizinhos` (deduzida dos boots vizinhos) e
  `sem_estimativa` (não foi possível estimar).
- `tipo` de alerta: `intersecao_vazia` (as âncoras do boot não são compatíveis entre si) e `deriva_excessiva`
  (a deriva do relógio do dongle passou do limite). Cada alerta novo também é gravado na tabela `alerta`.
- `tccelta_worker_fila_tempo` que só cresce indica que o worker não acompanha a ingestão.

## Reprocessamento

| Série | Tipo | O que mede |
|---|---|---|
| `tccelta_reprocessamento_linhas_total` | contador | Linhas regeneradas pelo comando de reprocessamento |

Aparece no processo que roda o reprocessamento (a CLI `tccelta`), não na API em execução.

## Métricas do KMS (`tccelta_kmssvc_*`)

O serviço KMS tem as suas próprias séries, em `kms:9300/metricas`, separadas das da API. Os nomes encontrados no
código são: `tccelta_kmssvc_http_requisicoes_total`, `tccelta_kmssvc_http_duracao_segundos`,
`tccelta_kmssvc_vault_chamadas_total`, `tccelta_kmssvc_vault_duracao_segundos`,
`tccelta_kmssvc_vault_disjuntor_aberto`, `tccelta_kmssvc_rejeicoes_total`, `tccelta_kmssvc_jwt_rotacoes_total` e
`tccelta_kmssvc_jwt_ultima_verificacao_ok`. Os rótulos e a semântica delas ficam para o `kms/docs`; não os revisei
aqui.

## Consultas úteis

Exemplos em PromQL. Não foram executados.

```promql
# Taxa de erro 5xx por rota, janela de 5 min
sum by (rota) (rate(tccelta_http_requisicoes_total{status=~"5.."}[5m]))
  / sum by (rota) (rate(tccelta_http_requisicoes_total[5m]))

# Latência p95 da ingestão de blocos
histogram_quantile(0.95, sum by (le) (rate(tccelta_http_duracao_segundos_bucket{rota="blocos"}[5m])))

# Blocos recusados por motivo
sum by (resultado) (rate(tccelta_blocos_total{resultado!~"aceito|duplicado"}[5m]))

# KMS indisponível
max(tccelta_kms_disjuntor_aberto) == 1

# Pool do banco saturado
max(tccelta_pool_conexoes_em_uso / tccelta_pool_conexoes_total) > 0.9
```
