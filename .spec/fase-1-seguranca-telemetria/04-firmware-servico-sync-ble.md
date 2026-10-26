# 04 — Firmware: serviço BLE SYNC (TLV, fragmentação e transporte)

## Objetivo

Criar o serviço GATT **SYNC**, separado do NUS, por onde passam as mensagens
binárias do protocolo (Tabela 3 do documento): `SYNC_RX` (WRITE, app →
dongle) recebe um TLV `T(1) | L(2) | V`; `SYNC_TX` (NOTIFY, dongle → app)
envia `C(1) + TLV` fragmentado em pedaços de até `MTU − 4` bytes, com o byte
de controle da Fig. 7 (bit 7 = primeiro fragmento; bits 6..0 = contador
cíclico 0..127). Esta spec fixa o formato de fio de todas as mensagens, o
codec puro, o enquadramento, o controle de fluxo das notificações no NimBLE e
a fila que leva o que chega do BLE até a `SyncTask` (spec 05). A semântica de
cada mensagem (o que o dongle faz) está na spec 05.

## Escopo

**Dentro:** GATT, anúncio, MTU, layouts byte a byte das mensagens 0x01–0x07,
`SyncProtocol` (codec puro), `Fragmenter` (puro), `BleSyncLink` (ESP32:
`ISyncLink` sobre NimBLE com créditos de notificação), mudanças em
`BleConnectivity` e `CollectorContext`, remoção do canal de dados antigo,
testes.

**Fora:** regras do handshake/cursor (spec 05); remontagem no app (spec 07);
LESC e permissões cifradas nas características (Fase 2 — só deixamos o
gancho).

## Estado atual (verificado no código)

| Peça | Onde | Hoje |
|---|---|---|
| GATT | `src/connectivity/ble/BleConnectivity.{h,cpp}` | Só o NUS (`6E400001…`): TX `NOTIFY`, RX `WRITE\|WRITE_NR`. A própria classe é o callback de RX e de servidor. `onConnect/onDisconnect` chamam `setConnectionCallback(bool)`. Anúncio com o UUID do NUS e nome na *scan response*. MTU não configurado (`CONFIG_BT_NIMBLE_ATT_PREFERRED_MTU` = 255 no `nimconfig.h`). |
| Canal de dados | `src/connectivity/IDataChannel.h`, `SerialDataChannel.h`, `README.md` | Não existe canal BLE; `SerialDataChannel` imprime em hexa. O README descreve um contrato antigo (`FrameDecoder`, quadros `len u16 \| msg`). |
| Protocolo antigo | `src/collector/DataProtocol.{h,cpp}` | `BATCH/ACK/TIME_*` com `FrameDecoder`. |
| Filas | `src/rtos/CollectorContext.{h,cpp}` | `uploadRx` (4) e `sessionRx` (2) de `DataMessage {bytes[64], len}`; bits `APP_CONNECTED`, `INTERNET`, `SESSION_OK`, `DIAG_MODE`, `BLOCKS_PENDING`; `connections` (contador atômico). |
| Conexão | `src/app/CollectorApp.cpp:36–44` | `onConnection`: `++connections`, liga/desliga `APP_CONNECTED` (desliga também `INTERNET` e `SESSION_OK`). |

API do NimBLE-Arduino conferida em `.pio/libdeps/base/NimBLE-Arduino/src` (pacote instalado: **2.5.0**, embora o `platformio.ini` peça `@2.5.1`; reconferir após `pio pkg install`, ver Fase 2 spec 01 R6):
`NimBLECharacteristic::notify(const uint8_t*, size_t, uint16_t connHandle)` →
`bool`; `NimBLEServer::getPeerMTU(uint16_t)`; `NimBLEDevice::setMTU(uint16_t)`;
`NimBLECharacteristicCallbacks::onWrite(chr, NimBLEConnInfo&)`,
`onSubscribe(chr, NimBLEConnInfo&, uint16_t subValue)`,
`onStatus(chr, NimBLEConnInfo&, int code)` — chamado em
`BLE_GAP_EVENT_NOTIFY_TX` para cada notificação enviada (código 0) ou com
erro; `NimBLEServerCallbacks::onMTUChange(uint16_t, NimBLEConnInfo&)`.
Pool de *mbufs*: `MYNEWT_VAL_MSYS_1_BLOCK_COUNT = 12` (ajustável por `CONFIG_BT_NIMBLE_MSYS1_BLOCK_COUNT`) × 256 B; `MAX_CONNECTIONS = 3`.

## Design

### 1. GATT

| Item | UUID | Propriedades | Tamanho máx. do valor | Observação |
|---|---|---|---|---|
| Serviço SYNC | `7A3C0001-9B1E-4F2A-8C5D-2E6F1A0B3C4D` | primário | — | Criado depois do NUS, mesmo servidor. |
| `SYNC_RX` | `7A3C0002-9B1E-4F2A-8C5D-2E6F1A0B3C4D` | `WRITE` | 64 B (`createCharacteristic(uuid, WRITE, 64)`) | **Um TLV completo por escrita**, com resposta. |
| `SYNC_TX` | `7A3C0003-9B1E-4F2A-8C5D-2E6F1A0B3C4D` | `NOTIFY` | `MTU − 3` por notificação | `C + pedaço do TLV`. |

**[Decisão] `SYNC_RX` só com `WRITE` (com resposta), sem `WRITE_NR`:** as
mensagens app → dongle são pequenas (≤ 50 B de TLV) e raras; a confirmação
de camada ATT dá ordem e entrega garantidas e permite *long write* (preparar/
executar) quando o MTU for menor que 53 B. A Fig. 7 não prevê byte de
controle no RX, coerente com isso. Escrita acima de 64 B é recusada pela
própria pilha (erro ATT de comprimento).

**[Decisão] Anúncio inalterado** (só o UUID de 128 bits do NUS + nome na
*scan response*): dois UUIDs de 128 bits (36 B) não cabem nos 31 B do
anúncio; o app já filtra pelo NUS e descobre o SYNC na conexão.

**Gancho da Fase 2:** as propriedades de cifra (`WRITE_ENC`/`READ_ENC`,
CCCD cifrado) entram nas duas características do SYNC exatamente nos pontos
de `createCharacteristic` desta spec.

### 2. MTU

- `NimBLEDevice::setMTU(517)` logo após `NimBLEDevice::init` (MTU preferido
  local; o efetivo é o mínimo entre os dois lados — Android pede 512/517,
  iOS negocia ~185–247).
- O MTU efetivo é lido **a cada mensagem** com
  `server->getPeerMTU(connHandle)`; `onMTUChange` só gera o evento
  `SYNC_MTU` (spec 06).
- Pedaço útil por notificação: `chunk = MTU − 4` (1 B de controle +
  `chunk` = `MTU − 3`, o máximo de uma notificação ATT). MTU mínimo 23 → 19 B.

### 3. TLV (os dois sentidos)

| Offset | Campo | Tipo | Tamanho |
|---|---|---|---|
| 0 | `T` (tipo) | u8 | 1 |
| 1 | `L` (comprimento de `V`) | u16 LE | 2 |
| 3 | `V` | bytes | `L` |

| T | Mensagem | Sentido no BLE | `L` válido | Formato de `V` |
|---|---|---|---|---|
| 0x01 | Desafio | app → dongle (SYNC_RX) | 45 (`flags` bit 0 = 0) ou 47 (bit 0 = 1) | §3.1 |
| 0x02 | Resposta | dongle → app (SYNC_TX) | 50 | §3.2 |
| 0x03 | Bloco | dongle → app (SYNC_TX) | `L = tamanho + T_tag`, em que `tamanho` (u16 LE do cabeçalho) conta a área de dados **com** o cabeçalho de 12 B: bloco de dados `17 + T … 1024 + T` (17 = cabeçalho + 1 PID); marcador `23 + T`; máx. 1088. (A Tabela 3 do `.tex` diz "55 a 1.056", incorreto: o mínimo real com `T = 32` é 49; a Fase 3 usa os mesmos limites.) | dados + tag (spec 02 §5, sem o CRC) |
| 0x04 | ACK | app → dongle (SYNC_RX) | 25 | §3.3 |
| 0x05 | Pedido de lote | app → dongle (SYNC_RX) | 2 | `K` u16 LE (1…65 535; 0 = ignorado) |
| 0x06 | Erro | ambos | 1 | código u8 (Tabela 5, spec 05 §6) |
| 0x07 | Aviso de novo bloco | dongle → app (SYNC_TX) | 13 | §3.4 |

#### 3.1 Desafio 0x01 — `V` com 45/47 B

| Offset | Campo | Tipo | Tam. |
|---|---|---|---|
| 0 | `c_S` | u32 LE | 4 |
| 4 | `N_S` | bytes | 16 |
| 20 | `ACK_seq` | u32 LE | 4 |
| 24 | `c_conf` | u32 LE | 4 |
| 28 | `flags_c` | u8 (bit 0 = modo diagnóstico; 1–7 reservados = 0) | 1 |
| 29 | `diag_min` | u16 LE (**só** se `flags_c` bit 0 = 1) | 2 |
| 29 ou 31 | `tag` | `HMAC-SHA256_{K_auth}("MALCONG"‖01‖DevId‖V[0..L−16))[0:16]` | 16 |

#### 3.2 Resposta 0x02 — `V` com 50 B

| Offset | Campo | Tipo | Tam. |
|---|---|---|---|
| 0 | `c_S` (eco) | u32 LE | 4 |
| 4 | `N_S` (eco) | bytes | 16 |
| 20 | `TAIL_seq` | u32 LE | 4 |
| 24 | `HEAD_seq` | u32 LE | 4 |
| 28 | `boot_id` | u16 LE | 2 |
| 30 | `t_ms` | u32 LE | 4 |
| 34 | `tag` | `HMAC-SHA256_{K_auth}("MALCONG"‖02‖DevId‖V[0..34))[0:16]` | 16 |

#### 3.3 ACK 0x04 — `V` com 25 B

| Offset | Campo | Tipo | Tam. |
|---|---|---|---|
| 0 | `ACK_seq` | u32 LE | 4 |
| 4 | `c_conf` | u32 LE | 4 |
| 8 | `flags_a` | u8 (bit 0 = âncora falhou, repetir handshake; 1–7 = 0) | 1 |
| 9 | `tag` | `HMAC-SHA256_{K_auth}("MALCONG"‖04‖DevId‖V[0..9))[0:16]` | 16 |

#### 3.4 Aviso de novo bloco 0x07 — `V` com 13 B

| Offset | Campo | Tipo | Tam. |
|---|---|---|---|
| 0 | `HEAD_seq` | u32 LE | 4 |
| 4 | `flags` | u8 (Tabela 4: bit 0 sem handshake nesta conexão; bit 1 sem handshake neste boot; bit 2 flash > 80 %; 3–7 = 0) | 1 |
| 5 | `DevId` | bytes | 8 |

Sem tag (o aviso é só um gatilho; nada nele é confiável para o servidor).

**Bits reservados.** Quem envia sempre os zera; quem recebe **ignora** bits
reservados ligados (`flags_c` 1–7, `flags_a` 1–7, aviso 3–7) — a tag continua
valendo e a mensagem é processada normalmente (compatibilidade futura).

Exemplos (chaves fixas da spec 01): desafio `c_S=7`
→ `01 2D00 07000000 000102…0F 64000000 06000000 00 5E08174F9D21CAA313D52A39DCD31943`;
ACK `ACK_seq=130, c_conf=7` → `04 1900 82000000 07000000 00 84EB2C1203B2720E492DB89E04F1FAE7`;
aviso `HEAD=130, flags=0x03` → `07 0D00 82000000 03 544343454C544101`.

### 4. `SYNC_RX`: regras de recepção no dongle

1. O `onWrite` (tarefa do NimBLE) **não interpreta** nada: copia até 64 B
   para um `SyncInput{kind=Rx}` e faz `xQueueSend(ctx.syncRx, …, 0)`. Fila
   cheia → descarta e incrementa `ctx.counters.syncRxDropped` (o app perde
   uma resposta e repete por *timeout*). **[Decisão]** O callback não emite
   evento (a pilha da tarefa do NimBLE é de 4 KB); a `SyncTask` compara o
   contador a cada `tick` e emite `SYNC_RX reason=queue_full` com a
   quantidade perdida.
2. A `SyncTask` valida: `len ≥ 3` e `L == len − 3`; `T ∈ {0x01, 0x04, 0x05, 0x06}`.
   Violação → descarta com evento `SYNC_RX ok=0 reason=…` (spec 06).
   **[Decisão]** não há código de erro "mensagem malformada" na Tabela 5;
   o dongle não responde nada e o app se recupera por *timeout*. Exceção:
   desafio/ACK com `L` inválido recebe erro 0x01 (spec 05), porque a tag não
   pode ser conferida e o app precisa pedir outro desafio.

### 5. `SYNC_TX`: enquadramento e fragmentação (Fig. 7)

Byte de controle `C`:

| Bits | Significado |
|---|---|
| 7 | 1 no primeiro fragmento de cada mensagem; 0 nos demais |
| 6..0 | Contador: 0 no primeiro fragmento, +1 a cada fragmento seguinte, `mod 128` |

Algoritmo (puro, `sync::Fragmenter`):

```
chunk = max(mtu, 23) − 4
for i, off = 0, 0; off < len; ++i, off += chunk:
    n = min(chunk, len − off)
    out[0] = (i == 0 ? 0x80 : 0x00) | (i & 0x7F)
    out[1..n] = msg[off .. off+n)
    emitir(out, n + 1)
```

- Uma mensagem sempre começa num fragmento novo; fragmentos de mensagens
  diferentes nunca se misturam (um só remetente, a `SyncTask`).
- Mensagens pequenas (aviso: 16 B; erro: 4 B; resposta: 53 B) cabem num
  fragmento com MTU ≥ 57 (C = `0x80`).
- Exemplo, resposta de 53 B com MTU 23 (`chunk` 19), 3 fragmentos:
  `80 02320007000000000102030405060708090A0B` /
  `01 0C0D0E0F4100000082000000030040E201003A` /
  `02 93F9DC93EDA9E7DBAD035426EEB061`.
- Bloco de 1 059 B (perfil `hmac`) com MTU 247: 5 fragmentos (4 × 243 + 87).

### 6. Arquitetura no firmware

```mermaid
sequenceDiagram
    participant NB as NimBLE host (núcleo 0)
    participant L as BleSyncLink
    participant Q as fila syncRx (8 × SyncInput)
    participant S as SyncTask (núcleo 0, prio 3)
    participant F as FlushTask (núcleo 1)
    NB->>L: onConnect / onDisconnect / onSubscribe / onWrite(SYNC_RX)
    L->>Q: SyncInput{Connected|Disconnected|Subscribed|Rx}
    F->>Q: SyncInput{Flushed, seq}  (timeout 0; cheia ⇒ noticePending)
    Q->>S: xQueueReceive
    S->>S: SyncEngine (spec 05)
    S->>L: send(TLV) — fragmenta e notifica
    L->>NB: notify(fragmento) [crédito]
    NB-->>L: onStatus(code) ⇒ devolve crédito
```

`SyncInput` (`rtos/CollectorContext.h`):

```cpp
namespace rtos {
enum class SyncInputKind : uint8_t { Connected, Disconnected, Subscribed, Unsubscribed, Rx, Flushed, InjectAck };
struct SyncInput {
    SyncInputKind kind;
    uint8_t  len;                         // Rx: bytes válidos em data
    uint16_t connHandle;                  // Connected/Rx
    uint32_t value;                       // Flushed: seq; InjectAck: ACK_seq; Connected/Subscribed/Unsubscribed/Rx: geração (ctx.connections)
    uint32_t tUs;                         // esp_timer (32 bits baixos) no callback: latência de fila
    uint8_t  data[sync::kSyncRxMax];      // 64
};                                        // 80 B
constexpr UBaseType_t kSyncRxDepth = 8;   // 640 B
}
```

Mudanças no `CollectorContext`:

| Removido | Adicionado |
|---|---|
| `uploadRx`, `sessionRx`, `DataMessage` | `QueueHandle_t syncRx` (8 × 80 B) |
| bits `SESSION_OK`, `BLOCKS_PENDING` | `std::atomic<bool> noticePending{false}` (Flush não conseguiu postar `Flushed`) |
| — | `Counters::syncRxDropped` (`std::atomic<uint32_t>`) |
| Mantidos: `APP_CONNECTED`, `INTERNET` (informativo, `AT#NET`), `DIAG_MODE`, `connections` | — |

**[Decisão]** A `SyncTask` não depende mais do bit `INTERNET`: quem decide
se há rede é o app (Fig. 14). `AT#NET` continua aceito e gera o evento `NET`.

### 7. `BleSyncLink` (`src/connectivity/ble/BleSyncLink.{h,cpp}`)

```cpp
namespace sync {
struct SendStats { uint16_t fragments; uint16_t mtu; uint32_t durUs; uint16_t retries; bool ok; };

/** Saída de mensagens TLV do SyncEngine. Só a SyncTask chama send(). */
class ISyncLink {
public:
    virtual ~ISyncLink() = default;
    virtual bool     send(const uint8_t* tlv, size_t len, SendStats& stats) = 0;
    virtual bool     ready() const = 0;      // conectado e com notificação habilitada
    virtual uint16_t mtu() const = 0;
};
}

class BleSyncLink final : public sync::ISyncLink {
public:
    explicit BleSyncLink(rtos::CollectorContext& ctx);
    void attach(NimBLEServer* server, NimBLECharacteristic* tx);

    // Chamados pela tarefa do NimBLE (via BleConnectivity):
    void onConnected(uint16_t connHandle);
    void onDisconnected();
    void onSubscribed(uint16_t subValue);
    void onRx(const uint8_t* data, size_t len, uint16_t connHandle);
    void onNotifyStatus(int code);

    bool     send(const uint8_t* tlv, size_t len, sync::SendStats& stats) override;
    bool     ready() const override;
    uint16_t mtu() const override;

private:
    static constexpr UBaseType_t kMaxCredits       = 4;     // teto; o efetivo depende do MTU (abaixo)
    static constexpr uint32_t    kCreditTimeoutMs  = 1000;  // < 2 s do app: o dongle aborta antes de o app desistir
    static constexpr int         kNotifyRetries    = 20;
    static constexpr uint32_t    kNotifyRetryMs    = 5;
    rtos::CollectorContext& _ctx;
    NimBLEServer*           _server = nullptr;
    NimBLECharacteristic*   _tx = nullptr;
    std::atomic<uint16_t>   _conn{BLE_HS_CONN_HANDLE_NONE};
    std::atomic<bool>       _subscribed{false};
    SemaphoreHandle_t       _credits;            // contador, máx. credits(mtu)
    uint8_t                 _frag[517 - 3];      // só a SyncTask usa
};
```

`send`:
1. `if (!ready()) return false;` `mtu = getPeerMTU(conn)`; `Fragmenter f(tlv, len, mtu)`.
2. Para cada fragmento: `xSemaphoreTake(_credits, 1000 ms)` (sem crédito →
   `ok=false`, aborta: o app vai detectar mensagem incompleta e mandar 0x04);
   `notify(_frag, n, conn)`; se `false` (sem *mbuf*: `BLE_HS_ENOMEM`), devolve
   o crédito, espera 5 ms e tenta de novo (até 20 vezes, contando `retries`);
   se esgotar, aborta.
3. `onNotifyStatus` (qualquer código) → `xSemaphoreGive(_credits)`.
   Contador com teto: créditos devolvidos depois de uma desconexão não
   estouram. **Só a sobrecarga nova** `onStatus(chr, NimBLEConnInfo&, int)`
   é sobrescrita: o `NimBLEServer.cpp` chama as duas (a antiga está
   *deprecated*) a cada `BLE_GAP_EVENT_NOTIFY_TX`; sobrescrever ambas
   devolveria dois créditos por notificação.
4. `onConnected`/`onDisconnected` e cada mudança de MTU recriam a janela
   (drena e devolve `credits(mtu)`), atualizam `_conn`/`_subscribed` e postam
   o `SyncInput`.

**[Decisão] Janela de notificações derivada do MTU e pool de *mbufs*
maior.** Um fragmento de `MTU − 3` bytes ocupa
`ceil((MTU − 3 + 20) / 236)` blocos `MSYS_1` de 256 B (cabeçalho de *mbuf*
incluso): 2 com MTU 247–270, 3 com MTU 517. O pool padrão do NimBLE-Arduino
é de 12 blocos (`MYNEWT_VAL_MSYS_1_BLOCK_COUNT`), compartilhado com a
recepção. Por isso:
- `[base]` do `platformio.ini` ganha `-DCONFIG_BT_NIMBLE_MSYS1_BLOCK_COUNT=24`
  (o nome do macro em `nimconfig.h` é `MSYS1`, sem `_` entre `MSYS` e `1`;
  `MSYS_1_BLOCK_COUNT` seria ignorado em silêncio). Custo: +3 KB de RAM.
- `credits(mtu) = clamp((poolBlocks − 4) / blocksPerFragment(mtu), 1, kMaxCredits)`
  → 4 com MTU 247 ou 517 e pool 24; 2 com MTU 517 e pool 12.
- Variável de benchmark: `-DSYNC_TX_MAX_CREDITS=<n>` (padrão 4) e o tamanho
  do pool, registrados no relatório (spec 06).

**[Decisão] Uma conexão por vez para o SYNC:** `_conn` guarda a última
conexão; uma segunda conexão simultânea (possível com `MAX_CONNECTIONS = 3`
até a Fase 2 limitar) substitui a anterior no SYNC e reinicia a sessão
(spec 05). Observação pré-existente: o NUS responde com `notify()` sem
`connHandle`, ou seja, para todas as conexões assinantes; a Fase 2 resolve
ao limitar a uma conexão/bond.

### 8. `BleConnectivity` (alterações)

- `begin()`: `NimBLEDevice::setMTU(517)`; cria o serviço SYNC (§1) e
  registra dois objetos de callback pequenos (`SyncRxCallbacks`,
  `SyncTxCallbacks`) que repassam ao `BleSyncLink`.
- `onConnect(server, connInfo)`: chama `_onConnection(true)` (existente) **e**
  `_syncLink->onConnected(connInfo.getConnHandle())`.
- `onDisconnect`: idem com `false`/`onDisconnected()`; reinicia o anúncio.
- `onMTUChange(mtu, connInfo)`: evento `SYNC_MTU`.
- Novo `void setSyncLink(BleSyncLink* link)` chamado em `startCollector` antes do `begin()`.

### 9. Codec puro (`src/sync/SyncProtocol.{h,cpp}`) e `Fragmenter`

```cpp
namespace sync {
enum class MsgType : uint8_t { Challenge = 0x01, Response = 0x02, Block = 0x03, Ack = 0x04,
                               BatchRequest = 0x05, Error = 0x06, Notice = 0x07 };
enum class ErrorCode : uint8_t { InvalidTag = 0x01, StaleChallenge = 0x02, InvalidAckSeq = 0x03,
                                 Incomplete = 0x04, NoSession = 0x05, ResendFromBoundary = 0x07 };

constexpr size_t kTlvHeaderSize    = 3;
constexpr size_t kHandshakeTagSize = 16;
constexpr size_t kChallengeLen = 45, kChallengeDiagLen = 47, kResponseLen = 50, kAckLen = 25;
constexpr size_t kBatchRequestLen = 2, kErrorLen = 1, kNoticeLen = 13;
constexpr size_t kMaxBlockValueLen = collector::kBlockSize + collector::kMaxTagSize;   // 1088
constexpr size_t kMaxTxMessage     = kTlvHeaderSize + kMaxBlockValueLen;               // 1091
constexpr size_t kSyncRxMax        = 64;

constexpr uint8_t kChallengeFlagDiag       = 0x01;
constexpr uint8_t kAckFlagAnchorFailed     = 0x01;
constexpr uint8_t kNoticeNoHandshakeConn   = 0x01;
constexpr uint8_t kNoticeNoHandshakeBoot   = 0x02;
constexpr uint8_t kNoticeFlashHigh         = 0x04;

struct TlvView { uint8_t type; const uint8_t* value; uint16_t len; };
bool   parseTlv(const uint8_t* buf, size_t len, TlvView& out);          // exige L == len − 3
size_t putTlvHeader(uint8_t type, uint16_t len, uint8_t out[3]);

struct Challenge { uint32_t cs; uint8_t nonce[16]; uint32_t ackSeq; uint32_t cConf;
                   uint8_t flags; uint16_t diagMin; size_t payloadLen; const uint8_t* tag; };
bool decodeChallenge(const uint8_t* v, size_t len, Challenge& out);     // len 45/47 coerente com flags

struct Response { uint32_t cs; uint8_t nonce[16]; uint32_t tailSeq; uint32_t headSeq;
                  uint16_t bootId; uint32_t tMs; };
size_t encodeResponsePayload(const Response& r, uint8_t out[34]);       // sem a tag

struct Ack { uint32_t ackSeq; uint32_t cConf; uint8_t flags; const uint8_t* tag; };
bool   decodeAck(const uint8_t* v, size_t len, Ack& out);               // len == 25
size_t encodeAckPayload(uint32_t ackSeq, uint32_t cConf, uint8_t flags, uint8_t out[9]);  // AT#ACK

bool   decodeBatchRequest(const uint8_t* v, size_t len, uint16_t& k);
size_t encodeNotice(uint32_t head, uint8_t flags, const uint8_t devId[8], uint8_t out[13]);

class Fragmenter {
public:
    Fragmenter(const uint8_t* msg, size_t len, uint16_t mtu);
    uint16_t count() const;
    bool next(uint8_t* out, size_t& outLen);    // false quando acabou
};
}  // namespace sync
```

O `SyncEngine` (spec 05) monta cada TLV num buffer próprio de
`kMaxTxMessage` (1 091 B, membro estático) e chama `ISyncLink::send`.

### 10. Remoções

| Arquivo | Motivo |
|---|---|
| `src/collector/DataProtocol.{h,cpp}` (+ `FrameDecoder`, `frameMessage`, `MessageType::Batch/Ack/Time*`) | Substituído por `sync/SyncProtocol` (TLV) e `Fragmenter`. BATCH vira o fluxo de blocos 0x03 com BOOT/LOSS. |
| `src/connectivity/IDataChannel.h`, `SerialDataChannel.h` | Substituídos por `sync::ISyncLink`/`BleSyncLink`. |
| `src/connectivity/README.md` | Reescrito com o contrato do SYNC (resumo desta spec + link para `docs/debug-events.md`). |
| `rtos::DataMessage`, filas `uploadRx`/`sessionRx` | §6. |

## Plano arquivo por arquivo

| Ação | Arquivo | Mudança |
|---|---|---|
| Criar | `src/sync/SyncProtocol.{h,cpp}` | §9 (codec). |
| Criar | `src/sync/Fragmenter.{h,cpp}` | §5. |
| Criar | `src/sync/ISyncLink.h` | `ISyncLink`, `SendStats`. |
| Criar | `src/connectivity/ble/BleSyncLink.{h,cpp}` | §7. |
| Alterar | `src/connectivity/ble/BleConnectivity.{h,cpp}` | §8; UUIDs `SYNC_SERVICE_UUID`, `SYNC_RX_UUID`, `SYNC_TX_UUID` como `constexpr const char*`. |
| Alterar | `src/rtos/CollectorContext.{h,cpp}` | §6. |
| Alterar | `src/app/CollectorApp.cpp` | `onConnection` deixa de mexer em `SESSION_OK`; cria `BleSyncLink` e chama `ble.setSyncLink`. |
| Alterar | `src/tasks/FlushTask.cpp` | Após `write` bem-sucedido: `SyncInput{Flushed, seq}` com timeout 0; falha → `ctx.noticePending = true`. Remove `BLOCKS_PENDING`. |
| Remover | §10 | — |
| Alterar | `platformio.ini` | `env:native`: `+<sync/>`. `[base]`: `-DCONFIG_BT_NIMBLE_MSYS1_BLOCK_COUNT=24`. Opcional nos envs de benchmark: `-DSYNC_TX_MAX_CREDITS=<n>`. |
| Criar | `test/test_sync_protocol/test_main.cpp` | Ver Testes. |

## Testes

Nativos (`pio test -e native -f test_sync_protocol`):
- `parseTlv`: aceita `L == len − 3`; recusa `len < 3`, `L` maior/menor.
- `decodeChallenge`: 45 B com `flags=0`; 47 B com `flags=1` e `diag_min`;
  45 B com `flags=1` → falso; 47 B com `flags=0` → falso; o `payloadLen` e o
  ponteiro da tag batem com os vetores da spec 01.
- `encodeResponsePayload` com os valores do vetor da spec 01 → os 34 B exatos.
- `decodeAck`/`encodeAckPayload` ida e volta; `encodeNotice` → `82000000 03 544343454C544101`.
- `Fragmenter`: o exemplo do §5 byte a byte; mensagem de exatamente `chunk`
  bytes → 1 fragmento; `chunk + 1` → 2; 1 091 B com MTU 23 → 58 fragmentos e
  contador `0x80, 0x01…0x39`; mensagem de 3 000 B (só teste) com MTU 23 →
  contador volta a `0x00` depois de `0x7F`; MTU < 23 tratado como 23.

Em placa (com nRF Connect no celular, antes do app existir):
- Descobrir o serviço `7A3C0001…` com RX `WRITE` e TX `NOTIFY`.
- Habilitar notificações no TX: chega um aviso 0x07 de 16 B com `80` na frente
  (`80 07 0D00 …`, spec 05).
- Escrever `05 0200 0A00` no RX (pedido de lote, `K=10`) sem handshake:
  chega `80 06 0100 05` (erro "sem sessão").
- Escrever 65 B no RX: a pilha recusa (erro ATT de comprimento) e nenhum
  `SYNC_RX` aparece no log.

## Passos de implementação

1. `SyncProtocol` + `Fragmenter` + testes nativos. *Checkpoint:* `test_sync_protocol` verde.
2. `CollectorContext` (fila nova) e remoção do canal antigo, deixando
   `SessionTask`/`UploadTask` fora do `startCollector` (a `SyncTask` vem na
   spec 05; até lá, uma `SyncTask` provisória que só responde avisos).
   *Checkpoint:* `pio run` em todos os envs.
3. `BleSyncLink` + `BleConnectivity`. *Checkpoint:* testes de placa com nRF
   Connect do bloco acima (o aviso e o erro 0x05 vêm do `SyncEngine` mínimo).
4. Vazão: enviar 200 blocos de 1 059 B seguidos para um Android com MTU 517;
   registrar `SYNC_TX dur_us` e `retries`. *Checkpoint:* nenhum `ok=0` sem
   desconexão; `retries` < 1 % dos fragmentos.

## Critérios de aceite

- Serviço SYNC com os UUIDs e propriedades do §1, coexistindo com o NUS.
- TLV e layouts byte a byte do §3 implementados pelo codec e cobertos por
  vetores; fragmentação do §5 com `MTU − 4` do MTU negociado por conexão.
- Nenhum processamento em callback do NimBLE além de copiar e enfileirar.
- Notificações com controle de fluxo por crédito; aborto limpo em
  desconexão ou em 1 s sem crédito (antes do *timeout* de 2 s do app).
- Canal de dados antigo (`DataProtocol`, `IDataChannel`, `SerialDataChannel`,
  filas `uploadRx`/`sessionRx`) removido; `connectivity/README.md` atualizado.

## Riscos e decisões em aberto

| Risco / ponto | Impacto | Recomendação |
|---|---|---|
| `onStatus` não chamado em algum caminho de erro do NimBLE | Créditos vazam e o envio trava 2 s por fragmento | Reabastecer créditos em `Connected`/`Disconnected` e quando a `SyncTask` fica 5 s ociosa sem notificação em voo. |
| iOS com MTU 185 e janela de conexão longa | Vazão menor | Aceitável (bloco a cada 6–10 s). Medir no benchmark (spec 06). |
| Escrita longa (`allowLongWrite`) não suportada em algum Android | Desafio de 50 B com MTU 23 falha | O app sempre pede MTU 512 antes; se o MTU < 53, registrar e tentar de novo após renegociar. |
| `MAX_CONNECTIONS = 3` até a Fase 2 | Outro celular derruba a sessão SYNC | Documentado; a Fase 2 limita a um bond. |
| **[Aberto]** Data Length Extension / PHY 2M | Pode dobrar a vazão | Fora do escopo funcional; incluir como variável do benchmark se sobrar tempo (`NimBLEDevice::setDefaultPhy`). |
