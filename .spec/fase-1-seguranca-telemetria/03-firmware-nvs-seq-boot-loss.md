# 03 — Firmware: reserva de `seq` na NVS, bloco BOOT e estado persistente

Estado implementado da recuperação de `seq` após perda de energia (Fig. 4 do
documento de segurança): a NVS guarda uma reserva `R` (`seq_rsv`) gravada como
`seq + N` a cada `N` blocos; todo boot grava um bloco BOOT com `seq = R` e
`[from_seq, to_seq] = [último seq na flash + 1, R − 1]`. O LOSS não é
persistido (é gerado no envio, ver `05`).

## Componentes

| Peça | Arquivo | Responsabilidade |
|---|---|---|
| `SeqReserve` | `src/collector/SeqReserve.{h,cpp}` | Contador de `seq` com reserva; `recover` e `afterRecovery` (puros). `SEQ_RESERVE_STEP` (padrão 64, 1..65 536) define `N`. |
| `ISeqReservePersistence` | `src/collector/SeqReserve.h` | `saveSeqReserve(uint32_t)`; implementada por `NvsStore`. |
| `NvsStore` | `src/storage/NvsStore.{h,cpp}` | `Preferences` no namespace `collector` da partição `nvs`; também implementa `datasync::ISyncStatePersistence` (`last_cs`). |
| `BlockWriter` | `src/collector/BlockWriter.*` | Consome a reserva antes de cada bloco (ver `02`). |
| `startCollector` | `src/app/CollectorApp.cpp` | Ordem de boot e gravação do BOOT. |

## Conteúdo da NVS (`nvs`, namespace `collector`)

| Chave | Tipo | Escrita por | Quando | Conteúdo |
|---|---|---|---|---|
| `config`, `config_ver` | blob, u8 (=2) | `CollectorControl` | `AT#PID`, `AT ST`, ... | Configuração da coleta. |
| `boot_id` | u16 | `nextBootId()` | uma vez por boot | Incrementa e **pula 0** (`0xFFFF → 1`): 0 identifica o LOSS. |
| `seq_rsv` | u32 | `BlockWriter` (via `saveSeqReserve`) | quando `seq >= reserva` | `R`: todo `seq` já gravado é `< R`. Ausente = 0. |
| `last_cs` | u32 | `SyncEngine` | a cada desafio aceito, antes de responder | Maior `c_S` aceito. Ausente = 0. |
| `last_seq` | u32 | — | — | Legado: lido uma vez na migração e removido. |
| `wipe_req` | u8 | `requestWipe()` (`AT#WIPE`) | antes do reinício | Pedido de *wipe* no próximo boot. |

Em `devsec`/`prod` a `nvs` é cifrada pela NVS encryption sem mudança de
código; no `dev` fica em claro (ver `01`).

## `SeqReserve`

```cpp
static SeqRecovery recover(uint32_t reserveInNvs, uint32_t legacyLastSeq, uint32_t newestInLog);
static SeqReserve  afterRecovery(const SeqRecovery&, uint32_t step = kSeqReserveStep); // next = R, reserva em RAM = 0
bool needsReserve(uint32_t seq) const { return seq >= _reserve; }
uint32_t reserveFor(uint32_t seq) const { return seq + _step; }
```

`recover`:

```
reserveInNvs > 0 : R = reserveInNvs;                               migrated = false
senão            : R = max(legacyLastSeq, newestInLog) + 1;         migrated = true
R = max(R, newestInLog + 1)                                         // nunca reusar seq
fromSeq = (newestInLog == 0) ? R : newestInLog + 1
toSeq   = R - 1
```

Invariante: todo `seq` gravado na flash é `< reserva persistida`. O BOOT
(`seq = R`) dispara `saveSeqReserve(R + N)`; o bloco `R + N` dispara
`saveSeqReserve(R + 2N)`, sempre **antes** do bloco que a consome.

## Sequência de boot (`startCollector`)

```mermaid
sequenceDiagram
    participant App as startCollector
    participant K as KeyStore
    participant N as NvsStore
    participant S as LittleFsBlockStore
    participant W as BlockWriter
    App->>App: eventLog.begin()
    App->>K: printBootLine(), load(keys) + cria IMessageAuth/IHandshakeAuth
    App->>N: begin(), loadConfig(), nextBootId() (pula 0)
    App->>S: begin(wipe)
    alt wipe (COLLECTOR_WIPE_ON_BOOT ou wipe_req)
        App->>N: clearSeqState()
    end
    App->>N: loadSeqReserve(), loadLegacyLastSeq()
    App->>App: rec = SeqReserve::recover(rsv, legacy, store.newestSeq())
    App->>W: BlockWriter(..., afterRecovery(rec), bootId)
    App->>W: writeBootMarker(rec.fromSeq, rec.toSeq)
    W->>N: saveSeqReserve(R + N)
    W->>S: startNewSegment(), append(BOOT seq=R)
    opt rec.migrated e BOOT gravado
        App->>N: removeLegacyLastSeq()
    end
    App->>App: eventos MARKER e BOOT; barramento, filas, BLE e tarefas
```

Todo boot grava um BOOT, inclusive o primeiro (`R = 1`, faixa vazia `[1, 0]`) e
quando não há furo (`[R, R − 1]`). Faixa vazia = `to_seq = from_seq − 1`. Se o
BOOT falhar, fica pendente e o `BlockWriter` o refaz antes de qualquer dado
(`02`).

## Quedas de energia

`n` = maior `seq` no log; `Rnvs` = reserva na NVS.

| Queda em... | NVS | Log | Próximo boot |
|---|---|---|---|
| Boot, antes de gravar `R + N` | `Rnvs = R` | `n < R`, sem BOOT | `R` de novo; BOOT `[n+1, R−1]` |
| Boot, entre `R + N` e o BOOT | `R + N` | `n < R` | `R' = R + N`; BOOT `[n+1, R+N−1]` |
| Bloco `s` gravado | `> s` | `n = s` | BOOT `[s+1, Rnvs−1]` |
| Durante o `append` de `s` | `> s` | `n = s − 1` (registro parcial descartado, segmento selado) | BOOT `[s, Rnvs−1]` |
| Durante a escrita de `seq_rsv` | valor antigo ou novo (atômica) | `n` < valor antigo | correto nos dois casos |

Invariantes: **R1** todo `seq` na flash `< reserva`; **R2** `seq` do log
estritamente crescentes entre boots; **R3** união dos `seq` do log com as
faixas dos BOOT cobre de 1 até o último `seq`.

Flash vazia com reserva `R > 1`: o BOOT sai com faixa vazia e o buraco é
declarado como LOSS no envio (em pedaços de até 64, ver `05`). Registros em RAM
(bloco aberto, até o intervalo de flush) perdidos numa queda não têm `seq`.

## `NvsStore`

| Método | Efeito |
|---|---|
| `nextBootId()` | Incrementa `boot_id`, pula 0, grava. |
| `loadSeqReserve()` / `saveSeqReserve(r)` | `seq_rsv`; `save` devolve `true` se gravou 4 bytes. |
| `loadLegacyLastSeq()` / `removeLegacyLastSeq()` | Migração de `last_seq`. |
| `clearSeqState()` | Remove `seq_rsv`, `last_seq`, `last_cs`, `wipe_req`. |
| `wipeRequested()` / `requestWipe()` | `wipe_req`. |
| `loadLastAcceptedCs()` / `saveLastAcceptedCs(cs)` | `last_cs` (ver `05`). |

## *Wipe* de bancada

`COLLECTOR_WIPE_ON_BOOT` (build) ou `AT#WIPE` (`wipe_req`, só com
`COLLECTOR_EVENTS`) formata o LittleFS e chama `clearSeqState()`: o próximo BOOT
tem `seq = 1` e qualquer `c_S` volta a ser aceito. O estado do dongle no
servidor (ACK, HEAD) precisa ser zerado junto ("Esquecer estado do dongle" no
servidor de bancada do app, ver `09`).

## Desgaste da NVS

Por bloco: 0 escritas. Reserva: 1 a cada `N` blocos + 1 por boot. `last_cs`: 1 por handshake aceito.

## Testes

`pio test -e native -f test_seq_reserve` (`esp32-firmware/test/test_seq_reserve/test_main.cpp`):

| Caso `(reserveInNvs, legacyLast, newestInLog)` | `(bootSeq, from, to, migrated)` |
|---|---|
| primeiro boot `(0, 0, 0)` | `(1, 1, 0, true)` |
| reboot normal `(192, 0, 130)` | `(192, 131, 191, false)` |
| reboot sem furo `(131, 0, 130)` | `(131, 131, 130, false)` |
| migração com log `(0, 127, 127)` | `(128, 128, 127, true)` |
| migração sem log `(0, 500, 0)` | `(501, 501, 500, true)` |
| reserva inconsistente `(100, 0, 150)` | `(151, 151, 150, false)` |

Mais: reserva gravada antes do `append` do bloco que a consome (N = 4);
`saveSeqReserve` falhando não grava nem consome `seq`; teste de propriedade
(semente fixa, 400 boots com N = 4, `append` falhando em ponto aleatório)
conferindo R1–R3.
