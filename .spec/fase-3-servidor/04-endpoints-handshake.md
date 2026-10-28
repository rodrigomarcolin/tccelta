# 04 — Endpoints do handshake (desafio e resposta)

Estado implementado: `internal/usecase/handshake/{issue_challenge,respond_challenge}.go`
(casos de uso), `internal/domain/handshake/{rules,types}.go` (regras puras),
`internal/api/{request_challenge,respond_challenge}.go` (HTTP) e as consultas de
`challenge.sql`, `state.sql`, `context.sql`.

## Contrato HTTP

| | `POST /v1/dongles/{id}/desafio` | `POST /v1/dongles/{id}/desafio/resposta` |
|---|---|---|
| `{id}` | DevId, 16 hex | idem |
| Requisição | sem corpo | `application/json`, até 2 KiB: `{"resposta":"<base64 do TLV 0x02>"}`; campos desconhecidos recusados |
| 200 | `{"desafio":"<base64 TLV 0x01>","expira_em":"<RFC 3339 UTC>"}` | `{"ack":"<base64 TLV 0x04>","token":"<JWT>","token_expira_em":"<RFC 3339>","token_validade_s":3600,"t_lat_s":60}` (`t_lat_s` omitido se `T_LAT_SUGERIDO` = 0) |
| Cabeçalhos | `Cache-Control: no-store` | idem |
| Erros | 400 `devid_invalido`; 403 `dongle_inativo`/`perfil_nao_permitido`; 404 `dongle_desconhecido`; 429 `desafios_pendentes` (com `Retry-After`), `limite_taxa`; 503 `kms_indisponivel` | 400 `corpo_malformado`/`base64_invalido`/`tlv_invalido`; 409 `desafio_invalido`/`c_s_antigo`; 413; 415; 422 `tag_invalida`/`resposta_inconsistente`; 429; 503 |

Base64 padrão com *padding* (`StdEncoding.Strict`). Os TLVs são os do SYNC sem o byte de controle: desafio `L` 45 (47 com diagnóstico), resposta `L` 50, ACK `L` 25; a tag de handshake tem 16 B (HMAC `K_auth` truncado).

## Sequência

```mermaid
sequenceDiagram
    participant A as App
    participant API as API
    participant K as KMS
    participant DB as PostgreSQL
    A->>API: POST /desafio
    API->>DB: Dongles.FindByDevID (cache) + VerifyAccess
    API->>DB: Challenges.Issue (c_S novo, N_S, S1, expira, MaxPending)
    API->>K: POST /v1/hmac (K_auth, fase pre)
    K-->>API: MAC completo (32 B)
    API-->>A: TLV 0x01 (tag = 16 primeiros bytes)
    Note over A: repassa ao dongle (BLE), recebe a Resposta 0x02
    A->>API: POST /desafio/resposta {resposta}
    Note over API: S2 = chegada (annotateS2), arredondado para cima ao microssegundo
    API->>DB: Challenges.Find (pré-checagem sem lock: nonce, não respondido, não expirado)
    API->>DB: States.Read (c_S > c_conf)
    API->>K: POST /v1/hmac (K_auth) e compara 16 B em tempo constante
    API->>K: POST /v1/jwt/sign (JWT, fora da transação)
    API->>DB: BEGIN; advisory lock do dongle
    API->>DB: Consume desafio (S2, boot_id, t_ms, HEAD, TAIL); releitura de dongle_state
    API->>DB: ApplyTail (OVERFLOW?), gap, boot, time_anchor, UpdateHandshake, Finalize, NOTIFY boot_pending
    API->>K: POST /v1/hmac (ACK, fase tx)
    API->>DB: COMMIT
    API-->>A: ack + token
```

## Emissão do desafio (`IssueChallenge`)

1. Dongle ativo e perfil permitido (`Dongle.VerifyAccess`: `none` só com `PERMITIR_PERFIL_NONE`).
2. `N_S` = 16 B de `crypto/rand`; `S1` = `now` truncado ao µs; `expira_em` = `S1 + DESAFIO_VALIDADE` (60 s).
3. `Challenges.Issue` insere o desafio com `c_S = max(último c_S + 1, segundos Unix)`, devolvendo também `ACK_seq` e `c_conf` do estado; recusa com `ErrPendingChallenges` se há `DESAFIO_MAX_PENDENTES` (3) desafios abertos (429, `Retry-After` = 1/3 da validade, mínimo 1 s).
4. Payload `c_S ‖ N_S ‖ ACK_seq ‖ c_conf ‖ flags` (`flags` = 0: diagnóstico não é emitido); tag = 16 primeiros bytes do HMAC `K_auth` sobre `TagInput(0x01, DevId, payload)`.

## Resposta (`RespondChallenge`)

Pré-checagens sem lock: tipo/tamanho do TLV; desafio existe, `N_S` igual (comparação em tempo constante), não respondido e não expirado em S2 (senão `desafio_invalido`); `c_S > c_conf` (senão `c_s_antigo`); tag `K_auth` (senão marca `invalid_tag`, `tag_invalida`). O JWT é emitido antes da transação.

Dentro da transação (advisory lock, `lock_timeout` 3 s):

| Passo | Efeito |
|---|---|
| Consume | marca o desafio respondido (`answered_at` = S2) e guarda `boot_id`, `t_ms`, `head_seq`, `tail_seq`; falha ⇒ `desafio_invalido` |
| Reconferência | relê `dongle_state`; `c_S ≤ c_conf` ⇒ `c_s_antigo` (rollback) |
| `ApplyTail` | `TAIL > HEAD + 1` ou `HEAD` < HEAD salvo ⇒ `resposta_inconsistente` (desafio finalizado como `inconsistent`, commit, HTTP 422). Se `TAIL > ACK + 1`: lacuna OVERFLOW `[ACK+1, TAIL−1]` e `ACK = TAIL − 1` |
| Âncora | RTT = S2 − S1; âncora só se `0 ≤ RTT ≤ ANCORA_RTT_MAX` (2 s); RTT negativo gera alerta `server_clock_regressed`. A geração do boot é inferida (`HandshakeGeneration`); o `t_ms` é desembrulhado (`timeline.Unwrap`) contra o último `t_ms` do boot; `Boots.EnsurePending` e `InsertAnchor` |
| ACK | `{ACK_seq, c_conf = c_S, flags}`; `flags` bit 0 = âncora falhou; tag via KMS (fase `tx`) |
| Estado | `UpdateHandshake` (ACK, HEAD, TAIL, `c_conf`, `last_ack`); `Finalize` (`accepted`/`accepted_no_anchor`, `ack_seq`, faixa de OVERFLOW, `token_jti`); `NOTIFY boot_pending` com `<dongle_id>:<geração>:<boot_id>` |

`c_conf` do ACK é o `c_S` desta resposta; `last_cs` do dongle e `last_c_s` do estado crescem monotonicamente (`c_S` emitido nunca regride, nem após reiniciar o app de bancada).

Dongle com contadores zerados (NVS/flash apagadas) é recusado com `resposta_inconsistente` (HEAD menor que o salvo) e deve ser reprovisionado como novo DevId.

## Regras puras (`domain/handshake`)

`ApplyTail(estado, tail, head) → (ack, perda?, erro)`, `EvaluateAnchor(s1, s2, max)`, `HandshakeGeneration(estado, bootID)` (`boot_id` 0 ou ≥ `last_boot_id` mantém a geração; menor avança), `TruncateMicro`, `MicroCeiling`. Erros: `ErrPendingChallenges`, `ErrInvalidChallenge`, `ErrStaleCS`, `ErrInvalidTag`, `ErrInconsistentResponse`.

## Concorrência

A pré-checagem corre sem lock; toda decisão que muda estado é refeita sob `pg_advisory_xact_lock(1, dongle_id)`. O JWT é assinado fora da transação; o ACK dentro (uma chamada ao KMS sob o lock). Erros `40P01`/`40001` repetem a transação uma vez (`database.InTransaction`); `lock_timeout` vira 503 `banco_ocupado`.

## Métricas e logs

`tccelta_handshakes_total{result}` (`challenge_issued`, `invalid_challenge`, `stale_c_s`, `invalid_tag`, `inconsistent`, `accepted`, `accepted_no_anchor`), `tccelta_anchor_rtt_seconds`, `tccelta_gaps_total{cause}`; logs `handshake.desafio_emitido`, `handshake.tag_invalida`, `handshake.inconsistente`, `handshake.aceito`.

## Testes

`usecase/handshake`, `domain/handshake`, `api` (testes unitários com `adapter/memory` e `port/porttest`), e os cenários `e2e`, `negative-protocol`, `time` e `generation` do simulador.
