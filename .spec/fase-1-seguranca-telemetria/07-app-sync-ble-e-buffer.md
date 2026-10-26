# 07 — App: canal SYNC, remontagem, buffer de blocos e coordenação da leitura contínua

## Objetivo

Fazer o app conversar com o serviço SYNC do dongle (spec 04) e executar o
lado app da **leitura contínua** (Fig. 15) e do **aviso de novo bloco**
(Fig. 14): remontar mensagens do SYNC_TX a partir dos fragmentos (byte de
controle, contador, salto, *timeout* de 2 s ⇒ erro 0x04), escrever TLVs no
SYNC_RX, pedir lotes com `K = min(HEAD − último lido, espaço livre)`, manter
um **buffer em memória** (até `B_max`) com a regra de envio avaliada a cada
segundo e a cada aviso/bloco/ACK (`T_lat` ou 32 blocos; lotes de até 32),
espelhar o cursor do dongle, repassar ACKs ao dongle e tratar os erros da
Tabela 5. O lado servidor (desafio, resposta, POST) passa pelo port da spec 09.

## Escopo

**Dentro:** extensão do port BLE (`services/`) e do adapter (`infra/`) para um
segundo par de características; modelos e regras puras em `domain/sync/`;
`SyncBleDatasource`; `DongleSyncRepository`; `SyncCoordinator`
(`application/sync/`, orquestra dois repositories + configurações); estado
observável para a UI (`SyncStatusViewModel` + cartão na aba "Mais"); testes.

**Fora:** contrato e implementações do servidor (spec 09); tela de
configurações e GPS (spec 08); HTTPS real (Fase 3); LESC (Fase 2); execução
em segundo plano com o app fechado.

## Estado atual (verificado no código)

| Peça | Onde | Hoje |
|---|---|---|
| Port BLE | `lib/src/services/ble/ble_service.dart` | `connect({deviceId, serviceUuid, rxUuid, txUuid, mtu})` → **um** par RX/TX. |
| Handle | `lib/src/domain/ble/ble_connection.dart` | `BleConnection { phase, currentPhase, incoming, isReady, write, disconnect }`; `BleConnectionPhase` com `reconnecting`. |
| Adapter | `lib/src/infra/ble/ble_plus.dart` | `_FbpConnection`: `connect(license: nonprofit, mtu: null)` → `requestMtu(512)` no Android → `discoverServices` → acha o NUS → `setNotifyValue(true)` → `onValueReceived` num `StreamController.broadcast` que **sobrevive** às reconexões; `write` usa `withoutResponse: true`. |
| Datasource | `lib/src/data/datasources/dongle_datasource.dart` | `DongleConfig` (UUIDs NUS); `connect(deviceId)`. |
| Provider | `lib/src/ui/connection/connection_providers.dart` | `dongleDatasourceProvider` = **`SecureDongleDatasource`** (exige PSK e embrulha a conexão em `EncryptedBleConnection`); `ScanScreen.select` empilha `/psk-setup` antes de conectar. O firmware atual **não** tem PSK. |
| Repository | `lib/src/data/repositories/dongle_repository_impl.dart` | Guarda a conexão, republica `connectionPhase`. |
| Precedente de repository sobre repository | `lib/src/data/repositories/obd2_repository_impl.dart` | `Obd2RepositoryImpl(DongleRepository)` cria `Elm327Client` + datasources sobre a conexão viva. |
| Dublês | `test/support/fake_ble_service.dart`, `scripted_ble_connection.dart`, `test/infra/ble/encrypted_ble_connection_test.dart` | Três implementações de `BleConnection` em teste. |
| Pacotes | `pubspec.yaml` | `flutter_blue_plus ^2.3.10`, `flutter_riverpod ^3.3.2`, `pointycastle`, `shared_preferences`, `dio`, `mocktail`; `fake_async` só transitivo. |

## Design

### 1. Port e adapter BLE com canais adicionais

`services/ble/ble_service.dart`:

```dart
/// Par RX/TX de um serviço GATT adicional, aberto na mesma conexão.
@immutable
class BleChannelSpec {
  const BleChannelSpec({
    required this.serviceUuid,
    required this.rxUuid,
    required this.txUuid,
    this.writeWithResponse = true,
  });
  final String serviceUuid;
  final String rxUuid;
  final String txUuid;
  /// Escrita com resposta (permite *long write*). @default true
  final bool writeWithResponse;
}

abstract interface class BleService {
  // ... adapterState, scan, stopScan inalterados
  Future<BleConnection> connect({
    required String deviceId,
    required String serviceUuid,
    required String rxUuid,
    required String txUuid,
    List<BleChannelSpec> extraChannels = const [],
    int mtu,
  });
}
```

`domain/ble/ble_connection.dart`:

```dart
/// Canal RX/TX adicional de uma [BleConnection]. [incoming] é estável entre
/// reconexões (a mesma stream continua emitindo depois de `reconnecting`).
abstract interface class BleChannel {
  Stream<List<int>> get incoming;
  bool get isReady;
  Future<void> write(List<int> bytes);
}

abstract interface class BleConnection {
  // ... membros atuais
  /// Canal pedido no `connect` por [serviceUuid]; `null` se não foi pedido.
  BleChannel? channel(String serviceUuid);
  /// MTU ATT negociado (23 enquanto desconhecido).
  int get mtu;
}
```

`infra/ble/ble_plus.dart` (`_FbpConnection`):
- Recebe `extraChannels`; cria **no construtor** um `_FbpChannel` por spec,
  cada um com seu `StreamController<List<int>>.broadcast()` (quem assinar
  cedo não perde notificação de nenhuma conexão).
- Em `_connectAndSetup`, depois do NUS: para cada canal, procura o serviço
  (ausente ⇒ canal fica `isReady = false`, **sem** falhar a conexão —
  firmware antigo continua funcionando com o NUS); acha RX/TX;
  **primeiro** `onValueReceived.listen(controller.add)` (com
  `device.cancelWhenDisconnected`) e **só depois** `setNotifyValue(true)`.
  O dongle envia o aviso 0x07 dentro do próprio `onSubscribe`, antes de o
  `setNotifyValue` resolver no app; como `onValueReceived` é *broadcast*,
  assinar depois perderia esse aviso. A mesma inversão vale para o TX do NUS
  (hoje `ble_plus.dart` assina depois).
- `write`: `c.write(bytes, withoutResponse: !spec.writeWithResponse, allowLongWrite: spec.writeWithResponse)`.
- `mtu`: `device.mtuNow`.
- Em queda: canais voltam a `isReady = false`; refeitos no `_connectAndSetup`.

Implementações de `BleConnection` que precisam acompanhar a interface:
`EncryptedBleConnection` (repassa `channel()`/`mtu` ao `inner`),
`FakeBleConnection` (`test/support/fake_ble_service.dart` — ganha canais
falsos programáveis), `ScriptedBleConnection` (`channel() => null`, `mtu => 247`)
e o `FakeBleConnection` local de `test/infra/ble/encrypted_ble_connection_test.dart`.

`data/datasources/dongle_datasource.dart`:

```dart
abstract final class SyncConfig {
  static const String service = '7A3C0001-9B1E-4F2A-8C5D-2E6F1A0B3C4D';
  static const String rx      = '7A3C0002-9B1E-4F2A-8C5D-2E6F1A0B3C4D';
  static const String tx      = '7A3C0003-9B1E-4F2A-8C5D-2E6F1A0B3C4D';
}
// connect(deviceId) passa extraChannels: const [BleChannelSpec(serviceUuid: SyncConfig.service, rxUuid: SyncConfig.rx, txUuid: SyncConfig.tx)]
```

**[Decisão] Transporte PSK legado desligado na Fase 1.** Sem contraparte no
firmware, o `SecureDongleDatasource` impede o próprio NUS de funcionar com
o dongle atual. `dongleDatasourceProvider` passa a usar `DongleDatasource`
quando `const bool.fromEnvironment('LEGACY_PSK_TRANSPORT')` for falso (padrão),
e `ScanScreen.select` vai direto para `/connecting` nesse caso. Nenhum código
PSK é apagado; a Fase 2 (LESC) decide o destino. **[Aberto]** confirmar com o
autor que desligar por padrão é aceitável até a Fase 2.

### 2. Regras puras (`lib/src/domain/sync/`)

Todas em Dart puro (`meta`, `dart:typed_data`), sem Flutter, testáveis sem
dublê de plataforma. Inteiros little-endian via `ByteData` (`Endian.little`).

| Arquivo | Conteúdo |
|---|---|
| `sync_protocol.dart` | `enum SyncMessageType { challenge(0x01), response(0x02), block(0x03), ack(0x04), batchRequest(0x05), error(0x06), notice(0x07) }`; `enum SyncErrorCode { invalidTag(0x01), staleChallenge(0x02), invalidAckSeq(0x03), incompleteMessage(0x04), noSession(0x05), resendFromBoundary(0x07) }`; constantes `kMaxSyncValueLength = 1088`, `kMaxBlocksPerPost = 32`, `kFragmentTimeout = Duration(seconds: 2)`, `kHandshakeStepTimeout = Duration(seconds: 5)`, `kBatchStallTimeout = Duration(seconds: 3)`, `kTokenRenewMargin = Duration(minutes: 5)`, `kRetryMin = Duration(seconds: 1)`, `kRetryMax = Duration(minutes: 5)`. |
| `sync_frame.dart` | `SyncFrame { type, value }`; `Uint8List toTlv()`; `static SyncFrame parseTlv(Uint8List)` (exige `L == len − 3`, lança `FormatException`). Fábricas `batchRequest(int k)`, `error(SyncErrorCode)`. |
| `dev_id.dart` | `DevId` (8 B imutáveis, `hex` maiúsculo de 16 dígitos, `==`/`hashCode`). |
| `new_block_notice.dart` | `NoticeFlags { noHandshakeConnection, noHandshakeBoot, flashHigh }` (bits 0–2); `NewBlockNotice { headSeq, flags, devId }` (`fromValue` exige 13 B). |
| `sync_block.dart` | `MarkerKind { none, boot, loss }`; `SyncBlock { seq, bootId, tMs, length, marker, fromSeq, toSeq, frame }` com `fromFrame` (lê `V[0..12)` e, se `length == 23` e `V[12] ∈ {0xFD, 0xFE}`, o marcador em `V[12..23)`). **Não** valida a tag. |
| `handshake_messages.dart` | `ChallengeInfo.fromValue` (`cS`, `ackSeq`, `cConf`, `flags`, `diagMin?`), `ResponseInfo.fromValue` (`cS`, `tailSeq`, `headSeq`, `bootId`, `tMs`), `AckInfo.fromValue` (`ackSeq`, `cConf`, `anchorFailed`). Só campos públicos; nunca verificam tag. |
| `fragment_assembler.dart` | §3. |
| `sync_buffer.dart` | §5. |
| `send_policy.dart` | §6. |
| `retry_backoff.dart` | `RetryBackoff { Duration next(); void reset(); }` — 1 s, 2 s, 4 s … teto 5 min, com *jitter* de ±10 % (`Random` injetável). |
| `sync_status.dart` | `SyncPhase { offline, waitingNotice, handshaking, handshakePending, open, resyncing, stopped }` e `SyncStatus` imutável (`copyWith`) com `phase`, `devId`, `headSeq`, `appAck`, `lastRead`, `buffered`, `inFlight`, `flags`, `nextAttemptAt`, `lastError`, `serverMode`, contadores de POST ok/falha. |

### 3. Remontagem (`FragmentAssembler`)

```dart
sealed class AssemblyOutcome {}
final class AssemblyPending extends AssemblyOutcome {}
final class AssemblyComplete extends AssemblyOutcome { final Uint8List tlv; }
final class AssemblyError extends AssemblyOutcome { final AssemblyErrorKind kind; }
enum AssemblyErrorKind { counterGap, continuationWithoutStart, startWhileAssembling, tooLong, overflow, timeout }

class FragmentAssembler {
  FragmentAssembler({this.maxValueLength = kMaxSyncValueLength, this.timeout = kFragmentTimeout});
  /// [now] é tempo monotônico (ex.: Stopwatch).
  List<AssemblyOutcome> add(List<int> fragment, Duration now);
  AssemblyOutcome? checkTimeout(Duration now);
  bool get assembling;
}
```

| Situação | Resultado |
|---|---|
| Fragmento vazio | Ignorado. |
| `C` bit 7 = 1, ocioso | Começa; contador deve ser 0 (senão `counterGap`); se já há ≥ 3 B, lê `L`; `L > maxValueLength` → `tooLong`. |
| `C` bit 7 = 1 durante uma montagem | `startWhileAssembling` **e** começa a nova mensagem com este fragmento (devolve as duas saídas). |
| `C` bit 7 = 0, ocioso | `continuationWithoutStart`. |
| Contador ≠ anterior + 1 (mod 128) | `counterGap`; descarta a montagem. |
| Bytes recebidos > `3 + L` | `overflow`. |
| Bytes recebidos = `3 + L` | `AssemblyComplete(tlv)`. |
| `now − último fragmento ≥ 2 s` durante montagem | `timeout` (via `checkTimeout`). |

Vetor de teste (do firmware, spec 04 §5): os três fragmentos
`80 02320007…0A0B`, `01 0C0D…E201003A`, `02 93F9…EB061` → resposta de 53 B.

### 4. `SyncBleDatasource` e `DongleSyncRepository`

`data/datasources/sync_ble_datasource.dart`:

```dart
sealed class SyncInbound {}
final class InboundFrame extends SyncInbound { final SyncFrame frame; }
final class InboundIncomplete extends SyncInbound { final AssemblyErrorKind kind; }

/// Envolve o canal SYNC: remonta fragmentos do TX e escreve TLVs no RX.
class SyncBleDatasource {
  SyncBleDatasource(this._channel, {MonotonicClock clock = const SystemMonotonicClock()});
  Stream<SyncInbound> get inbound;     // broadcast
  Future<void> send(SyncFrame frame);  // write(frame.toTlv()) com resposta
  void dispose();
}
```

Um `Timer.periodic(500 ms)` só enquanto `assembler.assembling` chama
`checkTimeout`. Uma mensagem remontada inteira mas com tipo desconhecido é
descartada com log (o dongle nunca a gera); ela **não** dispara erro 0x04,
porque o enquadramento estava íntegro.

`domain/repositories/dongle_sync_repository.dart`:

```dart
sealed class DongleSyncEvent {}
final class SyncLinkChanged extends DongleSyncEvent { final bool up; final int generation; }
final class NoticeReceived extends DongleSyncEvent { final NewBlockNotice notice; }
final class BlockReceived extends DongleSyncEvent { final SyncBlock block; }
final class ResponseReceived extends DongleSyncEvent { final SyncFrame frame; final ResponseInfo info; }
final class DongleErrorReceived extends DongleSyncEvent { final SyncErrorCode code; }
final class TransportIncomplete extends DongleSyncEvent { final AssemblyErrorKind kind; }

abstract interface class DongleSyncRepository {
  Stream<DongleSyncEvent> get events;
  bool get isLinkUp;
  int get mtu;
  Future<void> sendChallenge(SyncFrame challenge);   // valida tipo 0x01 e L 45/47
  Future<void> sendAck(SyncFrame ack);               // valida tipo 0x04 e L 25
  Future<void> requestBatch(int k);                  // 1..65535
  Future<void> sendError(SyncErrorCode code);        // só 0x04 e 0x07
}
```

`data/repositories/dongle_sync_repository_impl.dart`:
`DongleSyncRepositoryImpl(DongleRepository dongle)` (mesmo padrão do
`Obd2RepositoryImpl`): escuta `dongle.connectionPhase` e, **a cada fase**,
confere a identidade da conexão (`!identical(dongle.connection, _bound)`,
como o `_ensureDatasource` do `Obd2RepositoryImpl`); conexão nova → pega
`connection.channel(SyncConfig.service)` e cria o `SyncBleDatasource`, que
assina o `incoming` do canal. A fase `connecting` da primeira conexão não
chega ao repository (o adapter a emite dentro do `connect()`, antes de o
`DongleRepositoryImpl` assinar), mas isso não importa: o *controller* do
canal nasce no construtor do adapter e a assinatura em qualquer fase
anterior a `ready` não perde notificação. Em `ready` com canal pronto emite
`SyncLinkChanged(up: true, generation: ++n)`; em `reconnecting`,
`disconnected` ou `failed` emite `up: false`. Converte `InboundFrame` em
eventos tipados (falha de parse → log e descarta) e erros de escrita em
`SyncLinkFailure` (`core/errors/sync_failure.dart`).

### 5. Buffer (`SyncBuffer`)

```dart
@immutable
class BufferedBlock {
  const BufferedBlock({required this.block, required this.readAt});
  final SyncBlock block;
  final Duration readAt;     // monotônico
}

class SyncBuffer {
  SyncBuffer({required int capacity});
  int capacity;                         // B_max; reduzir não descarta nada já guardado
  int get length;
  int get freeSlots;                    // max(0, capacity − length)
  int get inFlightCount;
  BufferedBlock? get oldestPending;     // mais antigo fora de POST
  bool add(BufferedBlock b);            // false se seq já presente (duplicata) ou cheio
  List<BufferedBlock> takeForPost(int max);  // mais antigos fora de POST, em ordem; marca em voo
  void releaseInFlight();               // POST falhou: voltam a pendentes
  void removeUpTo(int ackSeq);          // remove seq ≤ ackSeq (em voo ou não)
  void dropPending();                   // descarta tudo que não está em POST
}
```

Ordenado por `seq` (`SplayTreeMap<int, _Entry>`). Blocos marcadores ocupam
uma vaga como qualquer outro.

### 6. Regra de envio (`SendPolicy`)

```dart
class SendPolicy {
  const SendPolicy({required this.tLat, this.maxPerPost = kMaxBlocksPerPost});
  final Duration tLat;
  final int maxPerPost;
  /// Quantos blocos postar agora (0 = não postar).
  int lotSize({
    required Duration now,
    required SyncBuffer buffer,
    required bool postInFlight,
    required bool flashHigh,
    required Duration? notBefore,       // espera de retry/429
    required int currentLotLimit,       // 32, ou reduzido após 429/5xx
  });
}
```

| Condição | Resultado |
|---|---|
| `postInFlight`, buffer sem pendentes ou `now < notBefore` | 0 |
| `pendentes ≥ maxPerPost` **ou** `flashHigh` **ou** `now − oldestPending.readAt ≥ tLat` | `min(pendentes, currentLotLimit, maxPerPost)` |
| senão | 0 |

**[Decisão]** O relógio de `T_lat` conta do instante em que o bloco entrou no
buffer (≈ aviso + leitura BLE, < 1 s), não do aviso: o app não associa avisos
a `seq` específicos.

### 7. `SyncCoordinator` (`lib/src/application/sync/sync_coordinator.dart`)

Use case de orquestração (combina `DongleSyncRepository`,
`SyncServerRepository` da spec 09 e `SyncSettingsRepository` da spec 08 —
justifica a camada `application/`, CLAUDE.md §5.1).

```dart
class SyncCoordinator {
  SyncCoordinator({
    required DongleSyncRepository dongle,
    required SyncServerRepository server,
    required SyncSettingsRepository settings,
    required HandshakeRunner handshake,           // spec 09
    MonotonicClock clock = const SystemMonotonicClock(),
    Duration tickInterval = const Duration(seconds: 1),
  });
  Stream<SyncStatus> get status;
  SyncStatus get current;
  void start();
  Future<void> dispose();
  Future<void> forceHandshake();                  // ação de depuração
}
```

Estado interno: `linkUp`, `generation`, `devId`, `headSeq`, `flags`,
`session ∈ {none, handshaking, open}`, `resyncing`, `lastRead`, `appAck`,
`buffer`, `awaitingBatch` (+ `batchTarget`, `lastBlockAt`), `postInFlight`,
`lotLimit` (32), `backoff`, `notBefore`, `openingAck` (ACK do último
handshake desta conexão), `rewroteOpeningAck`, `interruptedAt` (+ contagem).

**Espelho do cursor.** `lastRead` é o `cursor − 1` do dongle. Regra de
aceitação de bloco, com `expected = lastRead + 1`:

| Bloco recebido | Ação |
|---|---|
| `session ≠ open` | Descarta. |
| `resyncing` | Ver `resync` abaixo: aceita só o bloco-fronteira; os demais são descartados em silêncio. |
| `seq == expected` | Aceita; `lastRead = (LOSS) ? toSeq : seq`. |
| BOOT com `fromSeq == expected` (`seq > expected`) | Aceita (lacuna de boot); `lastRead = seq`. |
| `seq ≤ lastRead` | Duplicata: ignora (pode ser cópia de um bloco já em POST). |
| qualquer outro salto | Bloco perdido no transporte → `resync(incompleteMessage)`. |

**[Decisão]** O app lê só os campos públicos do cabeçalho e do marcador para
espelhar o cursor (inclusive o salto de um LOSS para `to_seq + 1`); isso não
dá a ele poder nenhum sobre os dados, que continuam autenticados pela tag.

**`resync(code)`** — usado para `0x04` (remontagem falhou, salto) e `0x07`
(buffer reiniciado): `sendError(code)`; `resyncing = true`;
`buffer.dropPending()`; `lastRead = appAck ?? lastRead`; em seguida
`requestBatch(k)` com o `k` recalculado (o dongle processa as escritas em
ordem, então esse 0x05 já parte de `ACK + 1`). **Fronteira:** enquanto
`resyncing`, o primeiro bloco aceitável em relação a `expected = appAck + 1`
(mesma regra da tabela acima: `seq == expected`, LOSS com `seq == expected`
ou BOOT com `fromSeq == expected`) encerra a ressincronização e é aceito;
qualquer outro bloco é descartado **sem** novo erro (no máximo um 0x04/0x07
por ressincronização). **[Decisão]** O aviso 0x07 não serve de fronteira:
um aviso de *flush* já enfileirado no dongle pode sair antes de o 0x04 ser
processado (spec 05 §4). Se nenhum bloco aceitável chegar em
`kBatchStallTimeout`, o pedido de lote é repetido (sem novo erro).

Eventos:

| Evento | Ação |
|---|---|
| `SyncLinkChanged(up)` | `up`: zera sessão/resync/lote/`openingAck`, `buffer.dropPending()` (o dongle reenvia do ACK), `lastRead = appAck`, fase `waitingNotice`. `down`: fase `offline`; o POST em voo continua (o ACK que voltar fica guardado e é escrito no próximo `up`). |
| `NoticeReceived(n)` | `devId`, `headSeq`, `flags`. Se `flags.noHandshakeConnection` ou `session ≠ open` → `drainThenHandshake()`; senão, se `flags.noHandshakeBoot` → `ensureHandshake()` sem bloquear lotes. Depois `maybeRequestBatch()` e `evaluateSend()`. |
| `BlockReceived(b)` | Regra de aceitação; `buffer.add(BufferedBlock(b, now))`; se `lastRead ≥ batchTarget` → `awaitingBatch = false`; `evaluateSend()`; `maybeRequestBatch()`. |
| `DongleErrorReceived(0x01 \| 0x02)` | Falha o handshake em curso (o `HandshakeRunner` também vê) → `ensureHandshake()` com *backoff*. |
| `DongleErrorReceived(0x03)` | Só log ("não reescrever aquele ACK"). |
| `DongleErrorReceived(0x05)` | `session = none`; se há `openingAck` e ainda não foi reescrito nesta conexão → `sendAck(openingAck)` uma vez; senão `drainThenHandshake()`. |
| `TransportIncomplete` | `resync(incompleteMessage)` (no máximo um erro por fronteira). |
| *tick* (1 s) | `evaluateSend()`; lote parado há `kBatchStallTimeout` → `awaitingBatch = false` e `maybeRequestBatch()`; handshake pendente vencido → `ensureHandshake()`. |

`maybeRequestBatch()`: se `linkUp && session == open && !resyncing &&
headSeq != null && lastRead != null && !awaitingBatch`:
`k = min(headSeq − lastRead, buffer.freeSlots, 65535)`; `k > 0` →
`requestBatch(k)`, `awaitingBatch = true`, `batchTarget = lastRead + k`.

`drainThenHandshake()`: **[Decisão]** se o buffer tem pendentes e o token
ainda vale, posta-os antes (mesma regra do `flashHigh`, lotes de 32 seguidos)
e só depois chama `ensureHandshake()`. Motivo: quando a sessão cai por flash
cheia, o handshake novo traz um `TAIL` maior e o servidor registra OVERFLOW
até `TAIL − 1` — blocos que o app já tinha lido nessa faixa seriam perdidos
sem necessidade.

`ensureHandshake()`: reaproveita o `Future` em curso; exige `devId` (senão
espera o próximo aviso); respeita `notBefore`; captura a `generation` atual e
chama `HandshakeRunner.run(devId, generation: g)` (spec 09). Se o resultado
chegar com `generation ≠ atual` (o link caiu e voltou durante o handshake),
é **descartado** (o ACK tem `c_conf` de uma Resposta de outra conexão e não
abriria a sessão). Sucesso → `openingAck = outcome.ack`,
`onAck(outcome.ack, writtenByRunner: true)`, `session = open`,
`backoff.reset()`. Falha → `notBefore = now + backoff.next()` (ou
`Retry-After`), fase `handshakePending`.

`onAck(frame)`: `a = AckInfo.fromValue(frame.value).ackSeq`;
`appAck = max(appAck, a)`; `buffer.removeUpTo(appAck)`;
`lastRead = max(lastRead, appAck)`; se não veio do *runner*,
`dongle.sendAck(frame)` (se o link estiver de pé; senão guarda para o
próximo `up`).

`evaluateSend()`: `n = policy.lotSize(...)`; `n > 0` → `post(n)`:
1. Se `server.needsHandshake(now, kTokenRenewMargin)` → `await ensureHandshake()`; falhou → sai.
2. `lot = buffer.takeForPost(n)`; `postInFlight = true`.
3. `result = await server.postBlocks(lot.map((b) => b.block.frame))`:

| Resultado (spec 09) | Ação |
|---|---|
| `BlocksAccepted(ack)` | `onAck(ack)`; se `ackSeq < lot.last.seq` (lote interrompido): `interrupted++` se o mesmo `ackSeq` interrompeu antes, senão 1; com 2 seguidas → `resync(resendFromBoundary)`. Senão `interrupted = 0`. `lotLimit = 32`, `backoff.reset()`. |
| `Unauthorized` (401) | Token descartado; `releaseInFlight`; `ensureHandshake()`; reavaliar. |
| `Forbidden` (403 outro dongle) | Idem 401 (bug do app: registrar). |
| `DongleBlocked` (403 inativo / 404 desconhecido) | Fase `stopped`, `lastError`; não repete até reconectar ou ação manual. |
| `RateLimited(retryAfter)` (429) | `releaseInFlight`; `notBefore = now + retryAfter`; `lotLimit = max(1, lotLimit ~/ 2)`. |
| `ServerUnavailable(retryAfter?)` (5xx/503/rede) | `releaseInFlight`; `notBefore = now + max(backoff.next(), retryAfter)`; `lotLimit` pela metade. |
| `MalformedRequest` (400) | `releaseInFlight`; `resync(resendFromBoundary)` (Fase 3, tabela de erros). |

4. `postInFlight = false`; `evaluateSend()`; `maybeRequestBatch()`.

Registro: cada decisão relevante sai num `debugPrint` estruturado
`SYNC <ms> <EVENTO> k=v …` (ex.: `SYNC 10512 BATCH_REQ k=62 last=130 head=192`),
no mesmo espírito dos eventos do firmware, para correlacionar pelo `seq`.
Só em `kDebugMode`.

### 8. DI e ciclo de vida (`lib/src/ui/sync/sync_providers.dart`)

| Provider | Tipo | Observação |
|---|---|---|
| `dongleSyncRepositoryProvider` | `Provider<DongleSyncRepository>` | Sobre `dongleRepositoryProvider`; não `autoDispose`. |
| `handshakeRunnerProvider` | `Provider<HandshakeRunner>` | Spec 09. |
| `syncCoordinatorProvider` | `Provider<SyncCoordinator>` | Cria, `start()`, `ref.onDispose(dispose)`. Instanciado pelo `ConnectionGuard` (`ref.watch`), que é global. |
| `syncStatusViewModelProvider` | `NotifierProvider<SyncStatusViewModel, SyncStatus>` | Escuta `coordinator.status`. |

UI: `lib/src/ui/sync/widgets/sync_status_card.dart` (composição de `CardButton`
+ `StatusBadge`) na `MoreScreen`, abaixo de "Criptografia": título
"Sincronização", subtítulo com a fase e `buffer n/B_max · ACK x · HEAD y`;
toque abre a tela de configurações da spec 08 (que mostra também o detalhe).

### 9. Tempo monotônico

`lib/src/core/time/monotonic_clock.dart`: `abstract interface class MonotonicClock { Duration now(); }`
e `SystemMonotonicClock` (um `Stopwatch` iniciado no construtor). Testes usam
`FakeMonotonicClock` + `fake_async` (adicionar `fake_async` às
`dev_dependencies`).

## Plano arquivo por arquivo

| Ação | Arquivo | Mudança |
|---|---|---|
| Alterar | `lib/src/services/ble/ble_service.dart` | `BleChannelSpec`; `extraChannels` no `connect`. |
| Alterar | `lib/src/domain/ble/ble_connection.dart` | `BleChannel`; `channel()`; `mtu`. |
| Alterar | `lib/src/infra/ble/ble_plus.dart` | §1. |
| Alterar | `lib/src/infra/ble/encrypted_ble_connection.dart` | Repassa `channel`/`mtu`. |
| Alterar | `lib/src/data/datasources/dongle_datasource.dart` | `SyncConfig`; `extraChannels` no `connect`. |
| Alterar | `lib/src/ui/connection/connection_providers.dart`, `lib/src/ui/connection/view/scan_screen.dart` | Transporte PSK legado atrás de `LEGACY_PSK_TRANSPORT` (§1). |
| Criar | `lib/src/domain/sync/{sync_protocol,sync_frame,dev_id,new_block_notice,sync_block,handshake_messages,fragment_assembler,sync_buffer,send_policy,retry_backoff,sync_status}.dart` | §2–6. |
| Criar | `lib/src/core/time/monotonic_clock.dart` | §9. |
| Criar | `lib/src/core/errors/sync_failure.dart` | `SyncLinkFailure extends Failure`. |
| Criar | `lib/src/data/datasources/sync_ble_datasource.dart` | §4. |
| Criar | `lib/src/domain/repositories/dongle_sync_repository.dart`, `lib/src/data/repositories/dongle_sync_repository_impl.dart` | §4. |
| Criar | `lib/src/application/sync/sync_coordinator.dart` | §7. |
| Criar | `lib/src/ui/sync/sync_providers.dart`, `ui/sync/view_model/sync_status_view_model.dart`, `ui/sync/widgets/sync_status_card.dart` | §8. |
| Alterar | `lib/src/ui/more/view/more_screen.dart`, `lib/src/ui/connection/widgets/connection_guard.dart` | Cartão; `ref.watch(syncCoordinatorProvider)`. |
| Alterar | `pubspec.yaml` | `dev_dependencies: fake_async`. |
| Alterar | `test/support/fake_ble_service.dart`, `test/support/scripted_ble_connection.dart`, `test/infra/ble/encrypted_ble_connection_test.dart` | Implementar `channel`/`mtu`; `FakeBleChannel`. |
| Criar | `test/support/{fake_ble_channel,fake_dongle,fake_sync_server_repository,fake_monotonic_clock}.dart` | `FakeDongle`: implementa `DongleSyncRepository` emulando o dongle (lista de blocos, cursor, ACK, erros) — reaproveita as regras da spec 05 em versão mínima. |
| Criar | Testes listados abaixo | — |

## Testes (`fvm flutter test`)

| Arquivo | Casos |
|---|---|
| `test/domain/sync/sync_frame_test.dart` | TLVs dos vetores da spec 01/04 (desafio, ACK, aviso) ida e volta; `L` incoerente lança. |
| `test/domain/sync/fragment_assembler_test.dart` | Vetor de 3 fragmentos; mensagem de 1 fragmento; `counterGap`; continuação sem início; início durante montagem (2 saídas); `tooLong` (`L = 1089`); `overflow`; *timeout* de 2 s; contador volta de 127 para 0. |
| `test/domain/sync/sync_block_test.dart` | BOOT e LOSS dos vetores (spec 02 §3) → `marker`, `fromSeq`, `toSeq`; bloco de dados de 23 B cujo tipo é PID → `none`. |
| `test/domain/sync/handshake_messages_test.dart` | `AckInfo`/`ResponseInfo`/`ChallengeInfo` dos vetores. |
| `test/domain/sync/sync_buffer_test.dart` | Capacidade, duplicata, `takeForPost` em ordem e marcação, `releaseInFlight`, `removeUpTo` (incl. em voo), `dropPending` preserva em voo, redução de `capacity`. |
| `test/domain/sync/send_policy_test.dart` | `T_lat` vencido; 32 pendentes; `flashHigh`; `notBefore`; `postInFlight`; `currentLotLimit` reduzido. |
| `test/domain/sync/retry_backoff_test.dart` | 1, 2, 4 … 300 s (com *jitter* fixo); `reset`. |
| `test/data/datasources/sync_ble_datasource_test.dart` | `FakeBleChannel` → frames; fragmentos + 2 s parados → `InboundIncomplete(timeout)`; `send` escreve o TLV exato. |
| `test/data/repositories/dongle_sync_repository_impl_test.dart` | `FakeBleService` com canal SYNC: aviso emitido pelo canal falso **antes** de o `setNotifyValue` resolver chega ao repository, inclusive na primeira conexão (sem ver a fase `connecting`); `reconnecting → ready` emite `down`/`up` com geração nova; canal ausente → link nunca sobe e o NUS segue funcionando. |
| `test/application/sync/sync_coordinator_test.dart` (`FakeDongle` + `FakeSyncServerRepository` + `fake_async`) | (1) aviso com bit 0 → handshake → ACK escrito → pedido `K = HEAD − ACK`; (2) blocos chegam e ficam no buffer até `T_lat`; POST de até 32; ACK remove e é escrito no dongle; (3) 40 blocos acumulados → POST de 32 imediato e outro de 8 após `T_lat`; (4) `B_max` cheio → para de pedir; ACK libera e retoma; (5) flash alta → POST sem esperar `T_lat`; (6) fragmento perdido → um único erro 0x04 seguido de pedido de lote; blocos antigos que ainda chegam são descartados sem novo erro; o primeiro bloco `seq = ACK + 1` encerra a ressincronização; (7) LOSS `[a,b]` seguido de `b+1` aceito; BOOT com lacuna aceito; salto sem marcador → 0x04; (8) 0x05 → reescreve o ACK de abertura uma vez, depois handshake; (9) 401 → handshake e reenvio; 429 → espera `Retry-After` e lote pela metade; 503 → *backoff* 1, 2, 4 s; (10) lote interrompido duas vezes no mesmo ACK → 0x07; (11) reconexão BLE → buffer pendente descartado, POST em voo concluído e ACK escrito depois; (12) token a < 5 min de expirar → handshake antes do POST; (13) link cai e volta durante o handshake → resultado da geração antiga descartado, novo handshake; (14) aviso com bit 0 (flash cheia) com 10 blocos pendentes e token válido → POST dos 10 antes do handshake. |
| `test/ui/sync/sync_status_card_test.dart` | Renderiza fases e números. |

## Passos de implementação

1. Port/adapter com canais + dublês atualizados. *Checkpoint:* `fvm flutter analyze` limpo e suíte atual verde.
2. Desligar o PSK legado por padrão. *Checkpoint:* app conecta ao firmware atual e o painel lê PIDs (bancada).
3. `domain/sync` puro + testes. *Checkpoint:* testes de domínio verdes.
4. `SyncBleDatasource` + `DongleSyncRepository`. *Checkpoint:* testes de data verdes; em bancada, o log `SYNC … NOTICE` aparece ao conectar.
5. `SyncCoordinator` (com o `HandshakeRunner` e o servidor de bancada da
   spec 09 já disponíveis). *Checkpoint:* `sync_coordinator_test` verde.
6. Providers, cartão e `ConnectionGuard`. *Checkpoint:* roteiro de bancada do README (leitura contínua) passa.
7. `fvm dart format .`, `fvm flutter analyze`, `fvm flutter test`.

## Critérios de aceite

- Conexão abre NUS e SYNC na mesma ligação GATT; ausência do SYNC não quebra o NUS.
- Remontagem conforme §3, com erro 0x04 em salto/continuação órfã/*timeout* de 2 s.
- Pedido de lote com `K = min(HEAD − último lido, livre)`; buffer até `B_max`;
  envio a cada `T_lat` ou 32 blocos (lotes ≤ 32), avaliado a cada segundo e a
  cada aviso/bloco/ACK; ACK remove do buffer e é escrito no dongle.
- Erros 0x01/0x02/0x03/0x05 do dongle tratados; 0x04/0x07 enviados nas
  situações da Tabela 5; ressincronização pelo primeiro bloco `seq = ACK + 1`, com no máximo um erro por ressincronização.
- Nenhum `view_model` fala com datasource; `flutter_blue_plus` só em `infra/`;
  análise `very_good_analysis` limpa; comentários em pt-BR.

## Riscos e decisões em aberto

| Risco / ponto | Impacto | Recomendação |
|---|---|---|
| **[Aberto]** Desligar o transporte PSK por padrão | Telas de PSK ficam sem efeito até a Fase 2 | Manter atrás de `--dart-define=LEGACY_PSK_TRANSPORT=true`; a Fase 2 remove ou reaproveita. |
| Android entrega notificações de duas características fora de ordem entre si | Nenhum (ordem só importa dentro do SYNC_TX) | — |
| App em segundo plano | iOS/Android podem suspender o BLE/timers | Fora do escopo; o dongle guarda tudo na flash. |
| Aviso perdido antes de o repository assinar | Atraso até o próximo aviso | `listen` antes de `setNotifyValue` no adapter (§1), vínculo por identidade da conexão (§4) e reenvio do firmware a cada 15 s sem sessão (spec 05). |
| `allowLongWrite` com MTU < 53 | Desafio não chega | MTU 512 pedido no Android; iOS ≥ 185. |
| `DongleSyncRepositoryImpl` depende de `DongleRepository` (repository sobre repository, contra a letra do `CLAUDE.md`) | Dívida de arquitetura, com precedente no `Obd2RepositoryImpl` | Aceito na Fase 1; resolver os dois depois com um port fino `ConnectionSource { BleConnection? current; Stream<…> changes }` consumido pelos datasources. |
| Buffer só em memória | App fechado perde blocos lidos e não confirmados | Sem perda real: o dongle só apaga com ACK e reenvia do ACK na próxima conexão. |
