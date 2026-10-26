# 06 — Firmware: eventos de depuração (v2) e benchmark com e sem segurança

## Objetivo

Registrar, no padrão de `docs/debug-events.md` / `collector/Events.h`
(`Event("NOME").num(...).hex(...)`, só com `COLLECTOR_EVENTS`), todo
processamento novo das specs 01–05, com **duração medida** e os
identificadores de correlação (`seq`, `c_s`, `type`, `len`, `tag_len`,
`frags`, `mtu`), para que o mesmo log permita (a) reconstruir passo a passo o
handshake, o cursor e a sequência de `seq`, e (b) medir o custo da segurança
comparando builds `SECURITY_PROFILE=none|hmac|asym`. Esta spec é o catálogo
único: as specs 01–05 só dizem *onde* emitir.

## Escopo

**Dentro:** catálogo de eventos novos/alterados/removidos, convenção de
unidade de tempo, mudanças em `Events.h`/`IClock`, envs de benchmark, plano
da versão 2 de `docs/debug-events.md`, receitas de reconstrução e de
benchmark, scripts de captura/análise, ajuste mínimo do `tools/event-viewer`.

**Fora:** métricas do app e do servidor (o app registra via `debugPrint` na
spec 07; o servidor tem métricas próprias na Fase 3).

## Estado atual (verificado no código)

- `src/collector/Events.h/.cpp`: `Event` com `num/flag/hex8/hex/hexList/token/text/absent/at`;
  buffer de 256 B ou externo (`kLargeCapacity = 2400`); linha truncada não é
  emitida; sem `COLLECTOR_EVENTS` vira classe vazia inline.
- `src/rtos/SerialEventLog.cpp`: *ring buffer* de 32 KB, tarefa `EventLog`
  (núcleo 0, prioridade 1, 4 KB); `STACK`/`HEAP` a cada 60 s; `LOG_DROP`.
- `src/collector/Clock.h`: `IClock::nowMs()` só em ms; `rtos::ArduinoClock`
  usa `millis()` (que no Arduino-ESP32 2.x é `esp_timer_get_time() / 1000`).
- `docs/debug-events.md` v1: `FORMAT v=1`; grupos 5.1–5.8; §5.7 com `CONN`,
  `NET`, `UP_SEND`, `UP_ACK`, `UP_REWIND`, `TIME_REQ`, `TIME_RESP`,
  `TIME_REPORT`; §7 formato do bloco sem tag.
- `tools/event-viewer/core.js`: categoria `link` com `UP_*`/`TIME_*`, estado
  `st.up`/`st.time` e gerador de demonstração com esses eventos.

## Design

### 1. Unidade de tempo

**[Decisão] Durações de processamento de segurança e de sincronização em
microssegundos, em campos com sufixo `_us`** (tipo novo `us` na gramática),
medidas com `esp_timer_get_time()`. Justificativa: um HMAC-SHA256 de 1 KB
com SHA em hardware leva da ordem de 50–150 µs, um CRC32 de 1 KB ~10–30 µs,
a verificação de uma tag de 16 B ~20 µs — com resolução de 1 ms quase tudo
leria `0`. Os campos `dur` existentes (ms) **não mudam** de unidade, para
não quebrar parsers v1; eventos novos nunca usam `dur` sem sufixo.

`IClock` ganha:

```cpp
virtual uint64_t nowUs() const { return static_cast<uint64_t>(nowMs()) * 1000u; }
```

`rtos::ArduinoClock::nowUs()` → `esp_timer_get_time()`; `FakeClock` ganha
`uint64_t us` controlável (avança junto com `advance`). Os `_us` são
diferenças no mesmo relógio que `t` (`t ≈ ⌊µs/1000⌋`).

Regra de medição: o intervalo medido termina **antes** de montar o evento;
formatação e fila do *logger* nunca entram em `_us`.

### 2. Gramática v2 (diferenças da v1)

- `FORMAT v=2`.
- Tipo `us`: inteiro decimal ≥ 0, microssegundos (`sign_us=84`).
- Tipo `hex` também usado para tags (64/128 dígitos) e CRC (`crc` = valor
  u32 em 8 dígitos, mais significativo primeiro: `crc=B294D366`; na flash
  fica `66 D3 94 B2`).
- Com `COLLECTOR_EVENTS_NO_HEX` (builds de benchmark), `hex` e `tag` do
  `FLUSH` saem como `-` (ausente) para reduzir a carga na serial.

### 3. Catálogo — eventos novos e alterados

"Onde" é informativo (arquivo/função que emite).

#### 3.1 Boot, chaves e configuração

| Evento | Campos | Significado | Onde |
|---|---|---|---|
| `FORMAT` | `v:int` | `v=2`. | `SerialEventLog::begin` |
| `KEYS` | `profile:enum(none,hmac,asym)` `source:enum(partition,dev_fixed,mac_derived,missing)` `dev_id:hex` `tag_len:int` `fe:enum(off,dev,release)` `sb:int` `load_us:us` | Chaves carregadas (spec 01 §7). `tag_len` = tag de bloco do perfil (0/32/64). `source=partition` = lida da `secnvs` por NVS encryption (todos os perfis). `fe`/`sb` = estado da *flash encryption*/Secure Boot (`dev` → `off`/0; `devsec` → `dev`/1; `prod` → `release`/1). Nunca mostra chave. | `CollectorApp` passo 0.5 |
| `BOOT` (alterado) | `boot:int` `wiped:bool` `rsv:int` `rsv_new:int` `legacy_last:int\|-` `boot_seq:int` `from:int` `to:int` `seq_next:int` `log_oldest:int` `log_newest:int` `lost:int` | Recuperação de `seq` (spec 03). `rsv` = reserva lida (0 = ausente); `rsv_new` = `R + N` gravada; `boot_seq` = `R`; `[from,to]` = faixa do BOOT (vazia se `to = from − 1`); `seq_next` = próximo após o BOOT. **Removido:** `nvs_last`. | `CollectorApp` passo 7 |
| `LOG_MIGRATE` | `removed:int` | Arquivos do log v1 (`/log`) apagados. | `LittleFsBlockStore::begin` |

#### 3.2 Flash e marcadores

| Evento | Campos | Significado | Onde |
|---|---|---|---|
| `FLUSH` (alterado) | `buf:int\|-` `kind:enum(data,boot)` `seq:int` `boot:int` `len:int` `tag_len:int` `ok:bool` `attempts:int` `reserved:bool` `reserve_us:us` `sign_us:us` `crc_us:us` `write_us:us` `dur:int` `crc:hex` `hex:hex\|-` `tag:hex\|-` | Gravação de um bloco (spec 02 §8). `buf=-` no BOOT. `reserved=1`: gravou `seq_rsv` antes (`reserve_us`). `dur` (ms) = total, como na v1. `hex` = dados (`len` B); `tag` = `tag_len` B. | `FlushTask::emitFlush`, `CollectorApp` (BOOT) |
| `MARKER` | `kind:enum(boot,loss)` `seq:int` `from:int` `to:int` `reason:enum(boot,crc,missing)` | Bloco marcador criado: BOOT no boot (antes do `FLUSH kind=boot`); LOSS no envio (spec 05 §7). | `CollectorApp`, `SyncEngine::sendNextBlock` |
| `SEG_NEW` | `first:int` `tag_len:int` `reason:enum(first,full,boot,sealed,tag_len)` | Segmento novo aberto. | `LittleFsBlockStore::append` |
| `SEG_SEAL` | `first:int` `last:int` `reason:enum(write_fail,scan_tail)` | Segmento selado (falha de escrita ou cauda inválida na varredura). | `LittleFsBlockStore` |
| `SEG_DROP` | (v1) `first:int` `last:int` `count:int` | Inalterado. Agora também alimenta `overflowUpTo`. | `LittleFsBlockStore::discardOldest` |

#### 3.3 Transporte SYNC

| Evento | Campos | Significado | Onde |
|---|---|---|---|
| `CONN` (alterado) | `up:bool` `gen:int` | Conexão BLE; `gen` = geração (`ctx.connections`). | `CollectorApp::onConnection` |
| `SYNC_SUB` | `on:bool` | App habilitou/desabilitou notificações no SYNC_TX. | `SyncEngine::onSubscribed` |
| `SYNC_MTU` | `mtu:int` | MTU negociado mudou. | `BleConnectivity::onMTUChange` |
| `SYNC_RX` | `type:hex` `len:int` `q_us:us` `ok:bool` `reason:enum(ok,short,bad_len,unexpected,queue_full)` | Mensagem recebida no SYNC_RX. `len` = bytes do TLV; `q_us` = espera na fila. `reason=queue_full` (com `len` = quantidade perdida desde o último `tick` e `type=-`) é emitido pela `SyncTask` a partir do contador `syncRxDropped`; o callback do NimBLE não emite eventos. | `SyncEngine::onMessage`, `SyncTask` (tick) |
| `SYNC_TX` | `type:hex` `len:int` `frags:int` `mtu:int` `dur_us:us` `retries:int` `ok:bool` | Mensagem enviada (todas as notificações). `dur_us` inclui espera por crédito. | `SyncEngine` após `ISyncLink::send` |
| `SYNC_ERR` | `dir:enum(tx,rx)` `code:hex` | Erro 0x06 enviado ao app (`tx`) ou recebido do app (`rx`). | `SyncEngine` |

#### 3.4 Handshake

| Evento | Campos | Significado | Onde |
|---|---|---|---|
| `HS_CHALLENGE` | `c_s:int` `last_cs:int` `ack:int` `c_conf:int` `flags:hex` `diag_min:int\|-` `len:int` `verify_us:us` `result:enum(ok,bad_len,bad_tag,old_cs,nvs_fail)` | Desafio processado. `verify_us` = recálculo + comparação da tag. | `SyncEngine::handleChallenge` |
| `HS_RESPONSE` | `c_s:int` `tail:int` `head:int` `boot:int` `t_ms:ms` `deleted_upto:int\|-` `nvs_us:us` `tag_us:us` `total_us:us` `q_us:us` | Resposta enviada. `nvs_us` = gravar `last_cs`; `tag_us` = tag da resposta; `total_us` = da retirada da fila até o fim do envio (processamento no dongle); `q_us` = fila. | `SyncEngine::handleChallenge` |
| `HS_ACK` | `ack:int` `c_conf:int` `flags:hex` `verify_us:us` `result:enum(ok,bad_len,bad_tag,bad_seq)` `advanced:bool` `opened:bool` `anchor_ok:bool` | ACK processado. `opened` = abriu a sessão; `anchor_ok` = `flags_a` bit 0 = 0. | `SyncEngine::handleAck` |
| `SYNC_SESSION` | `open:bool` `reason:enum(connect,ack,overflow,disconnect)` | Mudança de `sessionOpen`. | `SyncEngine` |

#### 3.5 Cursor, lotes e flash

| Evento | Campos | Significado | Onde |
|---|---|---|---|
| `SYNC_NOTICE` | `head:int` `flags:hex` `reason:enum(subscribe,flush,ack,resync,overflow,flash,retry)` | Aviso 0x07 enviado. | `SyncEngine::sendNotice` |
| `SYNC_REQ` | `k:int` `cursor:int` `head:int` `end:int\|-` `result:enum(ok,no_session,empty,ignored)` | Pedido de lote recebido. | `SyncEngine::handleBatchRequest` |
| `SYNC_BLOCK` | `seq:int` `kind:enum(data,boot,loss)` `len:int` `tag_len:int` `read_us:us` `crc_us:us\|-` `crc_ok:bool\|-` `sign_us:us\|-` `tx_us:us` `frags:int` `mtu:int` `cursor:int` | Um bloco enviado. `len` = `tamanho`; `crc_*` ausentes no LOSS gerado sem leitura; `sign_us` só no LOSS; `cursor` = valor depois do envio. | `SyncEngine::sendNextBlock` |
| `SYNC_BATCH_END` | `reason:enum(done,link,overflow,app_error,superseded,empty)` `sent:int` `bytes:int` `dur_us:us` | Fim de um lote: quantos blocos, quantos bytes de TLV, tempo do 1º envio ao fim. | `SyncEngine` |
| `SYNC_CURSOR` | `from:int` `to:int` `reason:enum(connect,ack,app_error,boot_gap,loss)` | Salto do cursor fora do "+1 por bloco". | `SyncEngine` |
| `SYNC_FREE` | `upto:int` `segs:int` `oldest:int` `dur_us:us` | Flash liberada após ACK/desafio (`deleteUpTo`). `oldest` = novo TAIL. | `SyncEngine::advanceAck` |
| `SYNC_OVERFLOW` | `upto:int` `ack:int` | Descarte por flash cheia atingiu `seq > ackLocal`; sessão invalidada. | `SyncEngine::checkOverflow` |

#### 3.6 Saúde

| Evento | Mudança |
|---|---|
| `STACK` | `task:enum(acquisition,flush,elm327,sync,logger)` — saem `session` e `upload`. |
| `HEAP`, `LOG_DROP` | Inalterados. |

### 4. Eventos removidos

| Evento v1 | Substituído por |
|---|---|
| `UP_SEND` | `SYNC_BLOCK` |
| `UP_ACK` | `HS_ACK` (+ `SYNC_FREE`) |
| `UP_REWIND` | `SYNC_CURSOR reason=app_error\|connect` |
| `TIME_REQ`, `TIME_RESP`, `TIME_REPORT` | `HS_RESPONSE` (o `t_ms` da Resposta é a âncora; S1/S2 ficam no servidor) |

`NET` permanece (informativo; `AT#NET` não condiciona mais nada).

### 5. Exemplo (perfil `hmac`)

```
EV 0 640 FORMAT v=2
EV 1 641 KEYS profile=hmac source=dev_fixed dev_id=544343454C544101 tag_len=32 fe=off sb=0 load_us=412
EV 2 655 MARKER kind=boot seq=192 from=131 to=191 reason=boot
EV 3 671 FLUSH buf=- kind=boot seq=192 boot=4 len=23 tag_len=32 ok=1 attempts=1 reserved=1 reserve_us=5210 sign_us=61 crc_us=4 write_us=9890 dur=15 crc=B294D366 hex=C00000000400… tag=E55ECDF1…
EV 4 671 BOOT boot=4 wiped=0 rsv=192 rsv_new=256 legacy_last=- boot_seq=192 from=131 to=191 seq_next=193 log_oldest=65 log_newest=192 lost=0
...
EV 52 9120 CONN up=1 gen=3
EV 53 9120 SYNC_SESSION open=0 reason=connect
EV 54 9120 SYNC_CURSOR from=65 to=65 reason=connect
EV 55 9480 SYNC_SUB on=1
EV 56 9481 SYNC_TX type=07 len=16 frags=1 mtu=247 dur_us=890 retries=0 ok=1
EV 57 9481 SYNC_NOTICE head=192 flags=03 reason=subscribe
EV 58 9902 SYNC_RX type=01 len=48 q_us=310 ok=1 reason=ok
EV 59 9908 HS_CHALLENGE c_s=1761480000 last_cs=1761479000 ack=130 c_conf=1761479000 flags=00 diag_min=- len=45 verify_us=38 result=ok
EV 60 9911 SYNC_FREE upto=130 segs=2 oldest=192 dur_us=2950
EV 61 9911 SYNC_CURSOR from=65 to=131 reason=ack
EV 62 9915 SYNC_TX type=02 len=53 frags=1 mtu=247 dur_us=1020 retries=0 ok=1
EV 63 9915 HS_RESPONSE c_s=1761480000 tail=131 head=192 boot=4 t_ms=9912 deleted_upto=130 nvs_us=5480 tag_us=36 total_us=12100 q_us=310
EV 64 10388 SYNC_RX type=04 len=28 q_us=290 ok=1 reason=ok
EV 65 10388 HS_ACK ack=130 c_conf=1761480000 flags=00 verify_us=35 result=ok advanced=0 opened=1 anchor_ok=1
EV 66 10388 SYNC_SESSION open=1 reason=ack
EV 67 10390 SYNC_TX type=07 len=16 frags=1 mtu=247 dur_us=850 retries=0 ok=1
EV 68 10390 SYNC_NOTICE head=192 flags=00 reason=ack
EV 69 10512 SYNC_REQ k=62 cursor=131 head=192 end=192 result=ok
EV 70 10513 SYNC_CURSOR from=131 to=192 reason=boot_gap
EV 71 10530 SYNC_TX type=03 len=58 frags=1 mtu=247 dur_us=1100 retries=0 ok=1
EV 72 10530 SYNC_BLOCK seq=192 kind=boot len=23 tag_len=32 read_us=9800 crc_us=3 crc_ok=1 sign_us=- tx_us=1100 frags=1 mtu=247 cursor=193
EV 73 10530 SYNC_BATCH_END reason=done sent=1 bytes=58 dur_us=17200
```

Leitura: depois de apagar até 130, o bloco mais antigo na flash é o BOOT
`seq = 192` com `[131, 191]`; o dongle informa `TAIL = 131` (spec 05 §4,
regra do TAIL efetivo), o servidor vê `TAIL = ACK_seq + 1` (sem OVERFLOW) e
depois aceita o BOOT por `from_seq = ACK_seq + 1`.

### 6. Receitas de reconstrução (novas seções 6.6–6.8 do documento)

**6.6 Sessão e handshake.** Por `gen` (de `CONN up=1` ao próximo `CONN up=0`):
sequência `SYNC_SUB → SYNC_NOTICE(flags bit0=1) → HS_CHALLENGE(ok) →
HS_RESPONSE → HS_ACK(opened=1) → SYNC_SESSION open=1`. Conferir: todo
`HS_RESPONSE` tem `c_s` > `last_cs` do `HS_CHALLENGE` imediatamente anterior;
nenhum `SYNC_BLOCK` antes de `SYNC_SESSION open=1` da mesma `gen`; todo
`SYNC_REQ result=no_session` é seguido de `SYNC_ERR dir=tx code=05`.

**6.7 Cursor.** Reconstruir `cursor` por `gen`: começa em `ack + 1` do
`SYNC_CURSOR reason=connect`; cada `SYNC_BLOCK` deve ter `seq` = cursor
reconstruído e `cursor` = `seq + 1` (dados/BOOT) ou `to + 1` do `MARKER kind=loss`
imediatamente anterior; saltos só com `SYNC_CURSOR`. Divergência = bug ou
evento perdido.

**6.8 Auditoria de `seq` (`seq_audit`).** Lista ordenada de `FLUSH ok=1`
(dados e BOOT) e dos `BOOT` de cada boot: (R2) `seq` estritamente crescente
globalmente; (R3) para cada boot, `from = último seq anterior + 1` e o
primeiro `FLUSH` do boot é `kind=boot seq=boot_seq`; todo inteiro entre 1 e o
último `seq` está num `FLUSH ok=1`, numa faixa `[from,to]` de BOOT ou numa
faixa `[from,to]` de `MARKER kind=loss` (furo com a flash vazia, spec 03 §4,
e corrompimentos). Perdas por flash cheia: `SEG_DROP` (e a lacuna OVERFLOW
fica no servidor).

### 7. Benchmark com e sem segurança

#### 7.1 Builds

| Env (`platformio.ini`) | Base | Flags extras |
|---|---|---|
| `collector_mock_bench_none` | `collector_mock_debug` | `-DSECURITY_PROFILE=0 -DCOLLECTOR_EVENTS_NO_HEX` |
| `collector_mock_bench_hmac` | `collector_mock_debug` | `-DSECURITY_PROFILE=1 -DCOLLECTOR_EVENTS_NO_HEX` |
| `collector_mock_bench_asym` | `collector_mock_debug` | `-DSECURITY_PROFILE=2 -DCOLLECTOR_EVENTS_NO_HEX` |
| `collector_twai_bench_{none,hmac,asym}` | `collector_twai_debug` | idem (carro/ECUSim) |

Seções reutilizáveis `[sec_none]`, `[sec_hmac]`, `[sec_asym]`, `[bench]` para
não repetir flags. Os envs `*_debug` continuam `hmac` com `hex` completo.

Todos os envs de benchmark são do perfil de hardware `dev` (spec 01 §8):
mesmo `framework = arduino, espidf`, mesmo `sdkconfig.defaults` e, portanto,
**NVS encryption ligada nos três perfis de `SECURITY_PROFILE`** — `none`
desliga só as tags de bloco/handshake, não a cifra da NVS. As durações de
NVS (`reserve_us`, `nvs_us`) incluem o AES-XTS em todos os perfis e não
entram na diferença "com × sem segurança"; registrar no relatório. Cada env
gera o próprio `sdkconfig.<env>` e compila o IDF inteiro (tempo de *build*
maior que no Arduino puro).

#### 7.2 Protocolo de medição

1. Mesma placa, mesmo celular (anotar modelo e MTU do `SYNC_MTU`), mesma
   config de PIDs/`Tmax` (`AT#LIST` registrado no início do log), mesmo pool
   de *mbufs* (`CONFIG_BT_NIMBLE_MSYS1_BLOCK_COUNT`) e `SYNC_TX_MAX_CREDITS`
   (variáveis de vazão, spec 04 §7).
2. Para cada perfil: `COLLECTOR_WIPE_ON_BOOT` uma vez (log limpo), reset do
   estado do dongle no servidor de bancada do app (spec 09), depois 30 min de
   coleta com o app em `T_lat = 60 s` (cenário **regime**).
3. Cenário **estresse**: `AT#FLUSH 1000` (um bloco por segundo) e
   `T_lat = 10 s`, 10 min.
4. Cenário **handshake**: 50 reconexões (o app tem ação de depuração
   "reconectar" ou `sync_bench.py --scenario handshake --repeat 50`).
5. Cenário **vazão**: desconectar o app por 20 min (acumula ~120–200
   blocos) e reconectar; medir o escoamento.
6. Captura: `python tools/event-bench/capture.py --port COMx --baud 921600 --out logs/<env>-<cenário>-<data>.log`
   (só grava linhas `EV `, com carimbo do PC).

#### 7.3 Receitas (seção 6.9 do documento)

| # | Métrica | Fórmula sobre os eventos | Comparação |
|---|---|---|---|
| B1 | Custo de assinatura por bloco | `FLUSH(kind=data, ok=1).sign_us`: média, p50, p95, máx.; e `sign_us / len` (µs/B) por faixa de `len` (≤256, ≤512, ≤768, ≤1024) | `hmac` vs `asym` (`none` ≈ 0) |
| B2 | Custo do CRC | `FLUSH.crc_us` (gravação) e `SYNC_BLOCK.crc_us` (leitura) | Igual nos perfis, cresce com `len + tag_len` |
| B3 | Overhead no *flush* | Δ de `mean(FLUSH.dur)` e de `mean(reserve_us + sign_us + crc_us + write_us)` entre perfil e `none`; fração `(sign_us + crc_us) / (sign_us + crc_us + write_us)` | A escrita na flash domina? |
| B4 | Handshake no dongle | `HS_RESPONSE.total_us` (média/p95) e partes `verify_us` (do `HS_CHALLENGE`), `nvs_us`, `tag_us`; `HS_ACK.verify_us` | `none` vs `hmac`/`asym` (iguais entre si) |
| B5 | Handshake ponta a ponta visto pelo dongle | `HS_ACK(opened=1).t − SYNC_NOTICE(reason=subscribe).t` (ms) e `HS_ACK.t − HS_RESPONSE.t` (ida e volta app + servidor) | Com servidor de bancada vs. HTTPS real (Fase 3) |
| B6 | Vazão BLE efetiva | Por lote: `SYNC_BATCH_END.bytes / dur_us` (B/µs → kB/s); por mensagem: `(len + tag_len + 3) / tx_us` agrupado por `mtu` | Perfis (tag maior = mais bytes) e MTU |
| B7 | Overhead de fragmentação | `frags` observado vs. `⌈(len + tag_len + 3) / (mtu − 4)⌉`; bytes de controle `frags / (len + tag_len + 3)` | Por MTU |
| B8 | Leitura + validação de bloco | `SYNC_BLOCK.read_us + crc_us` (média/p95) | Por perfil |
| B9 | Custo de segurança por bloco transmitido | `sign_us(FLUSH) + crc_us(FLUSH) + crc_us(SYNC_BLOCK)` (+ `sign_us` dos LOSS) | `hmac` vs `asym` vs `none` |
| B10 | Latência de confirmação | Para cada `seq`: `t(1º HS_ACK com ack ≥ seq) − t(FLUSH seq)` | Contra `L_max = T_bloco + T_lat + t_envio` (Eq. do documento) |
| B11 | Flash e descarte | `SYNC_NOTICE` com bit 2; `SEG_DROP`; `SYNC_OVERFLOW` | Cenário desconectado longo |
| B12 | Memória | `STACK task=flush/sync` mínimo; `HEAP min` | `asym` exige mais? |

`tools/event-bench/bench_report.py logs/*.log` produz `bench.csv` (uma linha
por evento relevante, colunas `env, cenário, evento, campos…`) e
`bench.md` com as tabelas B1–B12 (média, p50, p95, n). Dependências: só a
biblioteca padrão do Python 3.11.

### 8. Plano para `docs/debug-events.md` v2

| Seção | Mudança |
|---|---|
| Título e status | "format specification (v2)"; histórico curto v1 → v2. |
| §1 conceitos | + `K_auth`, `K_rec`, tag, CRC, perfil de segurança, SYNC, TLV, cursor, `ACK_seq`, `HEAD`/`TAIL`, BOOT/LOSS, sessão. |
| §2 habilitação | + `COLLECTOR_EVENTS_NO_HEX`, envs `*_bench_*`, `SECURITY_DEV_FIXED_KEYS`; `AT#ACK` (nova semântica), `AT#SYNC?`, `AT#CORRUPT`. |
| §3 gramática | Tipo `us`; `crc` em hexa (valor u32). |
| §4 tempo | Relação `t` (ms) × `_us` (mesmo relógio `esp_timer`). |
| §5.1 | `FORMAT v=2`, `KEYS`, `BOOT` alterado, `LOG_MIGRATE`. |
| §5.4 | `FLUSH` alterado, `MARKER`, `SEG_NEW`, `SEG_SEAL`. |
| §5.7 | Reescrita: "Connection, SYNC and handshake" com §3.3–3.5 desta spec; remove `UP_*`/`TIME_*`. |
| §5.8 | `STACK` com `sync`. |
| §6 | + 6.6, 6.7, 6.8 (§6 desta spec) e 6.9 (§7.3). §6.5 "Upload" substituída. |
| §7 | Formato do bloco v2: 4 tipos de registro, marcadores, registro na flash (`dados \| tag \| CRC`), segmento v2, TLV e byte de controle (resumo das specs 02 e 04). |
| §8 | Exemplo do §5 desta spec. |
| §9 limites | + "durações `_us` não incluem formatação de eventos"; "`hex` ausente em builds de benchmark". |

### 9. `tools/event-viewer` (ajuste mínimo)

- `core.js`: categoria `link` passa a listar `CONN, NET, SYNC_SUB, SYNC_MTU,
  SYNC_RX, SYNC_TX, SYNC_ERR, SYNC_NOTICE, SYNC_REQ, SYNC_BLOCK,
  SYNC_BATCH_END, SYNC_CURSOR, SYNC_FREE, SYNC_OVERFLOW, SYNC_SESSION`; nova
  categoria `security` (`KEYS, HS_CHALLENGE, HS_RESPONSE, HS_ACK, MARKER`);
  o reconstrutor troca `st.up`/`st.time` por `st.sync = {gen, open, ackLocal,
  cursor, head, flags}` alimentado por esses eventos; aceita `FORMAT v=1` e `v=2`.
- Gerador de demonstração: trocar o trecho `UP_*`/`TIME_*` por uma sessão
  como a do §5.
- **[Aberto]** Painel gráfico de benchmark no viewer. **Recomendação:** não
  na Fase 1; o `bench_report.py` cobre o TCC.

## Plano arquivo por arquivo

| Ação | Arquivo | Mudança |
|---|---|---|
| Alterar | `src/collector/Clock.h` | `nowUs()` com implementação padrão. |
| Alterar | `src/rtos/Adapters.h` | `ArduinoClock::nowUs()` com `esp_timer_get_time()`. |
| Alterar | `src/collector/Events.h/.cpp` | `kLargeCapacity = 2600`; helper `Event& us(const char* k, uint64_t v)` (alias de `num`, documenta a unidade); `kEventsHexEnabled` (falso com `COLLECTOR_EVENTS_NO_HEX`). |
| Alterar | `src/rtos/SerialEventLog.cpp` | `FORMAT v=2`; nomes de tarefa no `STACK`. |
| Alterar | Emissores das specs 01–05 | Conforme a coluna "Onde". |
| Alterar | `platformio.ini` | Seções `[sec_*]`, `[bench]` e envs do §7.1. |
| Alterar | `docs/debug-events.md` | §8 desta spec. |
| Criar | `esp32-firmware/tools/event-bench/capture.py`, `bench_report.py`, `README.md` | §7.2–7.3. |
| Alterar | `tools/event-viewer/core.js` (+ `index.html` se listar tipos) | §9. |
| Alterar | `test/support/Fakes.h` | `FakeClock::us`. |
| Alterar | `test/test_events/test_main.cpp` | Ver Testes. |

## Testes

Nativos (`test_events`, e os testes de cada spec com um `IEventSink` de teste):
- `FORMAT v=2`; tipo `us` formatado como inteiro; `FLUSH` com `hex` e `tag`
  de 64 B cabe em 2600 B (linha completa emitida); com `NO_HEX`, `hex=-`.
- Cada evento do catálogo aparece com os campos **na ordem da tabela** nos
  testes das specs 02, 03 e 05 (asserção por linha completa com `FakeClock`).
- `bench_report.py` com um log sintético (fixture em
  `tools/event-bench/tests/sample.log`): B1, B4, B6 e B10 com valores esperados.

Em placa: um log de 5 min do `collector_mock_debug` com o app passa no
`seq_audit` e na reconstrução de cursor (§6) sem divergências; os três envs
`*_bench_*` rodam o protocolo do §7.2 e o relatório sai completo.

## Passos de implementação

1. `IClock::nowUs`, `Events` (capacidade, `us`, `NO_HEX`), `FORMAT v=2`.
   *Checkpoint:* `test_events` verde.
2. Emitir os eventos junto com cada spec (01–05), sempre com teste de linha.
   *Checkpoint:* testes das specs verdes com as linhas do catálogo.
3. `docs/debug-events.md` v2. *Checkpoint:* revisão cruzada catálogo × código (grep dos nomes).
4. `capture.py` + `bench_report.py` + envs de benchmark. *Checkpoint:*
   relatório gerado para os três perfis no cenário estresse.
5. Viewer. *Checkpoint:* `index.html` abre um log v2 sem erros e mostra a categoria `security`.

## Critérios de aceite

- Todo processamento de segurança/sincronização emite evento com duração em
  `_us` e identificadores de correlação; nenhum evento novo usa `dur` em µs.
- `UP_*` e `TIME_*` não existem mais no código; `docs/debug-events.md` v2
  publicado e coerente com o código.
- Envs `*_bench_{none,hmac,asym}` compilam; `bench_report.py` produz B1–B12.
- Reconstruções 6.6–6.8 passam num log real de 5 min.

## Riscos e decisões em aberto

| Risco / ponto | Impacto | Recomendação |
|---|---|---|
| Carga da serial a 921 600 baud (FLUSH com 2 KB de hexa) | `LOG_DROP` e interferência na tarefa de log (núcleo 0) | `COLLECTOR_EVENTS_NO_HEX` nos envs de benchmark; *ring buffer* de 32 KB mantido. |
| Pilha da tarefa do NimBLE (4 KB) | Estouro se o callback fizer trabalho | Callbacks só copiam e enfileiram; nenhum evento emitido neles (spec 04 §4). |
| `COLLECTOR_EVENTS` altera o tempo total (formatação) | Benchmark enviesado | Os três perfis medidos com a mesma build de eventos; `_us` mede só o trecho de interesse. |
| Variância do SHA compartilhado e do NimBLE | p95 instável | Relatar p50/p95 com n ≥ 200 blocos por perfil. |
| **[Aberto]** Medir energia/consumo | Fora do log | Opcional com medidor USB; não é requisito do documento. |
