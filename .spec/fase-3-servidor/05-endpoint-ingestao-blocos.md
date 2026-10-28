# 05 — Ingestão de blocos e confirmação

Estado implementado: `internal/usecase/ingestion/{receive_batch,get_confirmation}.go`,
regras puras em `internal/domain/ingestion/apply.go`, HTTP em
`internal/http/{receive_batch,get_confirmation}.go`, reprocessamento em
`internal/usecase/reprocess/`.

## Contrato HTTP

### `POST /v1/dongles/blocos`

| Item | Valor |
|---|---|
| Autenticação | `Authorization: Bearer <JWT>` (ver `08`); o DevId vem do `sub` |
| Requisição | `Content-Type: application/octet-stream`; 1 a `LOTE_MAX_BLOCOS` (32) TLVs 0x03 concatenados; corpo máximo `32·1059 + 64` B |
| 200 | corpo `application/octet-stream` = TLV 0x04 (ACK, 28 B) |
| 422 | lote interrompido **sem progresso** por tag inválida: corpo também é o ACK (`octet-stream`) |
| 400 | `lote_malformado`, `tlv_invalido` |
| 413 / 415 | `corpo_grande` / `tipo_conteudo` |
| 429 | `lote_grande` (mais de 32 TLVs), `limite_taxa` |
| 503 | `kms_indisponivel`, `banco_ocupado`, `sobrecarga` |

Cabeçalhos de resposta (contrato de fio, em português):

| Cabeçalho | Valores |
|---|---|
| `X-Lote-Status` | `completo` \| `interrompido` |
| `X-Lote-Motivo` | só se interrompido: `tag_invalida`, `salto`, `marcador_invalido`, `pid_desconhecido`, `registro_malformado` |
| `X-Lote-Acao` | `nenhuma` \| `reler` (Erro 0x07 ao dongle) \| `aguardar` (`pid_desconhecido`, `registro_malformado`) |
| `X-Lote-Aceitos`, `X-Lote-Duplicados` | contagens |

Lote interrompido por outro motivo que não falta de progresso por tag inválida é 200 com o ACK parcial.

### `GET /v1/dongles/{id}/confirmacao`

Bearer com `sub` igual a `{id}` (senão 403 `token_outro_dongle`). Devolve o último ACK gravado (`dongle_state.last_ack`, `application/octet-stream`) sem chamar o KMS; 404 `sem_confirmacao` se nunca houve handshake aceito.

## Fluxo do lote

```mermaid
sequenceDiagram
    participant A as App
    participant API as API
    participant K as KMS
    participant DB as PostgreSQL
    A->>API: POST /blocos (TLVs 0x03)
    API->>API: JWT, limites, forma do corpo, DecodeBlock por perfil
    API->>DB: ReadSnapshot (contiguous_seq, c_conf, last_ack) - sem lock
    par verificação do lote (fase pre)
        API->>K: POST /v1/blocks/verify (K_rec, lotes de KMS_VERIFY_LOTE)
    and ACK provável
        API->>K: POST /v1/hmac (K_auth), salvo se igual ao last_ack
    end
    API->>DB: BEGIN; advisory lock; States.Read
    API->>API: ingestion.Apply(blocos, estado) - regras puras
    API->>DB: RegisterRejected; InsertRawBlocks; COPY reading/position; gaps; alertas
    API->>DB: Boots.UpdateIngestion + NOTIFY; UpdateIngestion(dongle_state, last_ack)
    opt ACK diferente do provável
        API->>K: POST /v1/hmac (fase tx)
    end
    API->>DB: COMMIT
    API-->>A: ACK + X-Lote-*
```

Passos de `ReceiveBatch.Run`:

1. Dongle ativo/perfil permitido; `ReadBatch` e `DecodeBlock` (tamanho `L = tamanho + T`); qualquer falha de forma rejeita o lote todo (400).
2. Snapshot sem lock. Só blocos com `seq > contiguous_seq` (sem repetir `seq`) vão ao KMS (`VerifyBlocks`, em fatias de `KMS_VERIFY_LOTE`, até `KMS_VERIFY_PARALELO` em paralelo) e, em paralelo, o ACK provável é pré-assinado (simulando todas as tags válidas), a menos que o `last_ack` já seja igual ao provável. Em regime, ≤ 1 ida e volta ao KMS antes da transação e nenhuma sob o lock quando o ACK não muda.
3. Transação (advisory lock, `lock_timeout` 3 s, `statement_timeout` 10 s): estado relido, `Apply`, gravações, e o ACK final reaproveita `last_ack` ou o pré-assinado; só reassina (fase `tx`) se divergir.
4. Atualiza `dongle_state` apenas se ACK, `max_seen_seq`, geração ou `last_ack` mudaram.

## Regras de aceitação (`ingestion.Apply`, pura)

`exp = contiguous_seq + 1`. Para cada bloco em ordem:

| Condição | Resultado |
|---|---|
| `seq < exp` | duplicado, ignorado sem interromper |
| tag inválida (bloco verificado) | interrompe (`invalid_tag`); **todo** bloco verificado com tag inválida e `seq ≥ exp` vai a `rejected_block` |
| registros malformados | rejeitado `malformed_record`, interrompe |
| BOOT (`0xFE`) | aceito se `to = seq − 1`, `from ≤ exp` e `seq ≥ exp`; lacuna `BOOT [exp, to]` se `exp ≤ to`; `exp = seq + 1`; atribui geração |
| LOSS (`0xFD`) | aceito se `from = exp` e `from ≤ seq ≤ to`; lacuna `LOSS [from, to]`; `exp = to + 1`; geração corrente |
| bloco de dados com `seq ≠ exp` | interrompe (`jump`) |
| PID fora do catálogo | rejeitado `unknown_pid`, interrompe |
| bloco de dados com `seq = exp` | aceito; `exp = seq + 1`; atribui geração |

`NewAck = exp − 1`; `max_seen_seq` = maior `seq` com tag válida. Geração do boot: `boot_id = 0` não participa; `boot_id` menor que o `last_boot_id` indica a volta do u16 e avança `boot_generation` (alerta `boot_generation_advanced`).

`Action`: `nenhuma` sem motivo; `aguardar` para `unknown_pid` e `malformed_record`; `reler` nos demais. Lote sem progresso = `NewAck` igual ao anterior com motivo `invalid_tag` (⇒ 422).

BOOT com CRC inválido chega do dongle como LOSS em pedaços (`[ACK+1, seq−1]` e `[seq, seq]`), aceitos pela regra acima. Não há saída manual para uma fila travada por bloco autêntico recusado.

## Gravação (`store`)

Para cada bloco aceito de dados: `t0` desembrulhado contra o último `t_ms` do boot (`timeline.Unwrap`, referência por geração G e G+1); `pid.Decode` gera `reading`/`position`; agregados por boot (`first_seq`, `last_seq`, `first_t_ms`, `last_t_ms`, `max_offset_ms` = `min(received_at − t_fim)`). `InsertRawBlocks` deve gravar todos (senão alerta `invariant_violated` e `ErrInvariant`); marcadores vão a `raw_block` com `marker`. Gaps com `block_seq`. O catálogo de PIDs fica em memória e é recarregado por `NOTIFY pid_changed`.

## Decodificação

Síncrona, dentro da transação do lote.

## Reprocessamento (`tccelta worker reprocess`)

Opções: `--dongle` (16 hex), `--from-seq` (1), `--to-seq` (0 = até o fim), `--batch` (200), `--apply` (sem isto é só ensaio). Roda como `tccelta_owner` (no compose, serviço `ferramentas`): relê blocos de dados de `raw_block` em janelas, apaga `reading`/`position` do intervalo (por `block_seq` e `received_at`, inclusive em chunks comprimidos), redecodifica com o catálogo atual e reinsere; marca `gold_pending` nos boots afetados. Make: `make reprocessar DONGLE=… [DESDE=] [APLICAR=1]`. Métrica `tccelta_reprocessing_rows_total`; alerta `reprocessing`.

## Métricas

`tccelta_batches_total{status}` (`completo`, `interrompido`, `sem_progresso`), `tccelta_blocks_total{result}` (`accepted`, `duplicate`, `invalid_tag`, `unknown_pid`, `invalid_marker`, `malformed_record`, `discarded_after_interruption`), `tccelta_ingestion_tx_duration_seconds`, `tccelta_inserted_rows_total{table}`, `tccelta_gaps_total{cause}`, `tccelta_kms_calls_total{...,phase}`.
