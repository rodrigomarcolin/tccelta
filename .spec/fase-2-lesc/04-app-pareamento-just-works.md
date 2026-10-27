# 04 — App: pareamento LESC Just Works fim a fim

> Fase 2 · spec 4 de 4. Depende do contrato de firmware das specs 01 (flags,
> motivos de desconexão, dados de fabricante) e 02 (janela). Pode ser
> desenvolvida em paralelo ao firmware usando o `FakeBleService`.

## 1. Objetivo

Levar o usuário do "achei o dongle" até uma sessão **cifrada e com bond**,
deixando o pareamento por conta do sistema operacional (Just Works), e tratar
todos os desvios: janela fechada ("segure o botão do dongle por 5 s"), janela
expirada, pareamento recusado, bond perdido (Android: remover e parear de
novo; iOS: orientar a esquecer nos Ajustes) e reconexão automática com bond
existente. Ao mesmo tempo, retirar do caminho de conexão a camada de
criptografia de aplicação por PSK, que não tem mais contraparte no firmware.

## 2. Escopo

Dentro: port/adapter BLE (bond, dados de fabricante, fase de segurança,
classificação de erros), datasource/repository, use cases de conexão segura
e "esquecer", `ConnectingViewModel` e telas de `ui/connection/`, rotas,
remoção da camada PSK do caminho de conexão, testes com `FakeBleService`.

Fora: protocolo SYNC e telas de parâmetros de sincronização (Fase 1), HTTPS
(Fase 3), firmware (specs 01–03).

Regras do `mobile-app/CLAUDE.md` que esta spec segue: cadeia
datasource → repository → view_model (sem atalho); use case em
`application/` só quando combina repositories; `flutter_blue_plus` só em
`infra/ble/ble_plus.dart`; domínio puro (só `meta`); comentários genéricos em
pt-BR; átomos de `ui/core/widgets/` reaproveitados; `fvm dart format .`,
`fvm flutter analyze`, `fvm flutter test` antes de concluir.

## 3. Estado atual do código (verificado)

| Item | Onde | Situação |
|---|---|---|
| Port BLE | `mobile-app/lib/src/services/ble/ble_service.dart` | `adapterState`, `scan({withServiceUuids, withNames, timeout, continuousUpdates})`, `stopScan`, `connect({deviceId, serviceUuid, rxUuid, txUuid, mtu})`. Nada de bond. |
| Adapter | `mobile-app/lib/src/infra/ble/ble_plus.dart` | `FlutterBluePlusBleService` + `_FbpConnection`: `connect(license: License.nonprofit, timeout: 12 s, mtu: null)` → `requestMtu` (Android) → `discoverServices` → `setNotifyValue(true)` → `ready`; queda após `ready` ⇒ `reconnecting` e uma nova tentativa; `write` sempre **sem resposta**. `_toDevice` ignora `manufacturerData`. |
| Domínio BLE | `lib/src/domain/ble/{ble_connection,ble_device,ble_adapter_state,ble_signal_level,last_dongle,security_mode}.dart` | `BleConnectionPhase` = idle, connecting, optimizingLink, discovering, enablingNotify, ready, reconnecting, disconnected, failed. `BleDevice{id,name,rssi,signal}`. |
| Datasource | `lib/src/data/datasources/dongle_datasource.dart` (`DongleConfig`, `DongleDatasource`) e `secure_dongle_datasource.dart` (`SecureDongleDatasource`) | O provider usa o **Secure**: sem PSK salva, toda conexão falha (`BleConnectionFailure('PSK não configurada…')`). |
| Repository | `lib/src/data/repositories/dongle_repository_impl.dart`, interface em `lib/src/domain/repositories/dongle_repository.dart` | Guarda a conexão, re-publica fases, mapeia erro → `BleScanFailure`/`BleConnectionFailure` (`lib/src/core/errors/ble_failure.dart`). |
| DI | `lib/src/ui/connection/connection_providers.dart` | `bleServiceProvider` → `FlutterBluePlusBleService`; `dongleDatasourceProvider` → `SecureDongleDatasource(…, settingsServiceProvider)`; `lastDongleRepositoryProvider`; `forgetDongleUseCaseProvider`; `selectedDongleProvider`. |
| ViewModel de conexão | `lib/src/ui/connection/view_model/connecting_view_model.dart` | `ConnectingState{phase, prep}`; `connect(id)`; ao `ready` roda `_prepare()` (ELM327) e `_rememberDevice()` (`LastDongle`). |
| Telas | `lib/src/ui/connection/view/{ble_permissions,bluetooth_off,scan,connecting,connected,connection_lost}_screen.dart`; widgets `connection_guard.dart`, `connection_state_view.dart`, `connection_background.dart` | `ScanScreen.select` empurra **`AppRoutes.pskSetup`**. `BlePermissionsScreen._advance` reconecta direto se há `LastDongle`. `ConnectionGuard._flowRoutes`/`_adapterOffExcludedRoutes` contêm `pskSetup`. |
| Rotas | `lib/src/router/app_routes.dart`, `app_router.dart` | `settings` → `PskSettingsScreen()`; `pskSetup` → `PskSettingsScreen(setupFlow: true)`; `cameraPermissions`; `pskScanQr`. |
| Use cases | `lib/src/application/connection/forget_dongle_use_case.dart` | Desconecta + apaga `LastDongle`; não mexe em bond. |
| "Mais" | `lib/src/ui/more/view/more_screen.dart` | Card "Criptografia" (PSK) → `AppRoutes.settings`; card "Esquecer dispositivo". |
| Camada PSK | `lib/src/core/crypto/{hkdf,psk_cipher}.dart`, `lib/src/infra/ble/{encrypted_ble_connection,ble_security_handshake}.dart`, `lib/src/services/settings/settings_service.dart`, `lib/src/ui/settings/**`, camera permissions (`data/datasources/camera_permissions_datasource.dart`, `data/repositories/camera_permissions_repository_impl.dart`, `domain/repositories/camera_permissions_repository.dart`) | Sem contraparte no firmware (`connectivity/secure/` não existe). Dependências só dela: `pointycastle`, `mobile_scanner`. |
| Plataforma | `android/app/src/main/AndroidManifest.xml`; `ios/Runner/Info.plist` | `BLUETOOTH_SCAN`, `BLUETOOTH_CONNECT` (bastam para `createBond`/`removeBond`), `CAMERA`; `NSBluetoothAlwaysUsageDescription`, `NSCameraUsageDescription`. |
| Testes | `test/support/fake_ble_service.dart` (`FakeBleService`, `FakeBleConnection`), `test/ui/connection/connecting_view_model_test.dart` (salva PSK fake para passar no gate) | — |
| Fase 1 | `.spec/fase-1-seguranca-telemetria/` | Acrescenta o SYNC à conexão e T_lat/B_max/T_GPS à tela de configurações. Conferir no código o que já entrou antes de executar §5.7. |

### 3.1 API do `flutter_blue_plus` 2.3.10 (conferida no pub cache)

`%LOCALAPPDATA%\Pub\Cache\hosted\pub.dev\flutter_blue_plus-2.3.10\lib\src\bluetooth_device.dart`
e `flutter_blue_plus.dart`:

| API | Plataforma | Comportamento verificado |
|---|---|---|
| `Future<void> createBond({int timeout = 90, Uint8List? pin})` | **Android** (iOS lança `androidOnly`) | Exige conectado; espera `bondState != bonding`; lança `createBondFailed` se terminar diferente de `bonded`; se já `bonded`, retorna sem esperar. O README recomenda chamá-lo logo após conectar para evitar o "diálogo duplicado" do Android. |
| `Future<void> removeBond({int timeout = 30})` | **Android** | Lança `removeBondFailed` se não chegar a `none`. iOS: `androidOnly`. |
| `Stream<BluetoothBondState> get bondState` / `prevBondState` | **Android** | Valor inicial + mudanças (`none`, `bonding`, `bonded`). |
| `static Future<List<BluetoothDevice>> get bondedDevices` | **Android** | Lista de bonds do SO. |
| `DisconnectReason? get disconnectReason` | ambas | `(platform, code, description)` da última queda. |
| `AdvertisementData.manufacturerData` | ambas | `Map<int, List<int>>` com a chave = Company ID (little-endian já decodificado). |
| `startScan({withServices, withRemoteIds, withMsd, …})` | ambas | Filtros são **OU** entre si. |

## 4. Comportamento de bonding por plataforma

| Tema | Android | iOS |
|---|---|---|
| Quem inicia o pareamento | O app, com `createBond()` logo após o `connect` quando não há bond. O dongle **só** manda *Security Request* a quem ele já tem bond (spec 01 D21), para pedir a cifração na reconexão. | A primeira operação GATT que receba erro de segurança: a inscrição no TX (`setNotifyValue`) recebe ATT 0x05 e o iOS mostra o alerta. Não há API para o app. Na reconexão com bond, o *Security Request* do dongle faz o iOS cifrar. |
| Interface do sistema | Pedido de consentimento (diálogo ou notificação, conforme versão/fabricante) [verificar nos aparelhos da bancada]. | Alerta "Solicitação de Emparelhamento Bluetooth" com "Emparelhar"/"Cancelar". |
| Saber se há bond | `bondState`. | Impossível: o app usa o próprio registro (`LastDongle`), gravado só depois de uma sessão autorizada. |
| Remover bond | `removeBond()` (usa API oculta por reflexão; [verificar] em Android 14/15; se falhar, orientar Ajustes › Dispositivos conectados). | Só o usuário: Ajustes › Bluetooth › (i) ao lado do dongle › Esquecer este dispositivo. |
| Reconexão com bond | O SO cifra com a LTK ao receber o *Security Request*. | Idem. |
| Dongle apagou os bonds | A cifração falha (dongle sem LTK): queda com `0x06` (*PIN or Key Missing*) ou `0x3D` (*MIC failure*), ou o dongle recusa com `0x05`; alguns Androids reiniciam o pareamento sozinhos [verificar]. | `CBError.peerRemovedPairingInformation` (código 14); o iOS não volta a cifrar com o dongle até o usuário esquecer o dispositivo. |
| Id do dongle (`remoteId`) | MAC público do dongle (estável). | UUID atribuído pelo iOS (estável para o mesmo dongle no mesmo iPhone [verificar] após esquecer e parear de novo). |

## 5. Design

### 5.1 Domínio (puro)

| Arquivo | Conteúdo |
|---|---|
| `domain/ble/ble_bond_state.dart` (novo) | `enum BleBondState { none, bonding, bonded, unknown }` — `unknown` = SO não informa (iOS). |
| `domain/ble/ble_link_error.dart` (novo) | `enum BleLinkError { pairingRejected, pairingTimeout, notAuthorized, bondLost, bondLostIos, remoteReset, busy, timeout, other }` com doc de cada caso (tabela §5.3). |
| `domain/ble/dongle_pairing_hint.dart` (novo) | `DonglePairingHint{windowOpen, hasBond}` + `static DonglePairingHint? fromManufacturerData(Map<int, List<int>> data)`: procura a chave `0xFFFF`, exige `bytes.length >= 2`, `bytes[0] == 0x01`; `windowOpen = bytes[1] & 0x01 != 0`, `hasBond = bytes[1] & 0x02 != 0`; senão `null` (firmware antigo ou outro aparelho). Constantes `companyId`, `formatVersion`. |
| `domain/ble/ble_device.dart` | Novo campo `final Map<int, List<int>> manufacturerData` (padrão `const {}`), em `copyWith` e na igualdade (comparação profunda com *helper* privado; o estado da janela muda sem mudar id/nome/rssi). Continua genérico (BLE puro): quem interpreta é o `DonglePairingHint`. |
| `domain/ble/ble_connection.dart` | `BleConnectionPhase` ganha `securing` (entre `connecting` e `optimizingLink`): "estabelecendo o enlace cifrado; o SO pode mostrar o pedido de pareamento". `BleConnection` ganha `BleLinkError? get lastError` (motivo do último `failed`, nulo se não houve). Atualizar a doc da enum (não citar "Fase 1/Fase 2", termos que passam a nomear fases do projeto). |

### 5.2 Port `BleService`

Acréscimos (assinaturas):

```dart
/// Se o SO permite ao app consultar, criar e remover bonds (Android).
bool get canManageBonds;

/// Estado do bond do SO com [deviceId]. Sem suporte: [BleBondState.unknown].
Future<BleBondState> bondState(String deviceId);

/// Remove o bond do SO com [deviceId]. Sem suporte: lança [UnsupportedError].
Future<void> removeBond(String deviceId);

/// `scan` ganha [withRemoteIds] (filtro por id de plataforma; filtros são "OU").
Stream<List<BleDevice>> scan({
  List<String> withServiceUuids,
  List<String> withNames,
  List<String> withRemoteIds,
  Duration timeout,
  bool continuousUpdates,
});

/// `connect` ganha [securingTimeout]: prazo para o pareamento/cifração
/// (inclui o tempo do usuário no diálogo do SO).
Future<BleConnection> connect({
  required String deviceId,
  required String serviceUuid,
  required String rxUuid,
  required String txUuid,
  int mtu,
  Duration securingTimeout,
});
```

### 5.3 Adapter `ble_plus.dart`

`FlutterBluePlusBleService`:

- `canManageBonds => Platform.isAndroid`.
- `bondState(id)`: Android → `BluetoothDevice.fromId(id).bondState.first` mapeado; iOS → `unknown`.
- `removeBond(id)`: Android → `device.removeBond()`; iOS → `UnsupportedError`.
- `_toDevice` preenche `manufacturerData: r.advertisementData.manufacturerData`.
- `scan` repassa `withRemoteIds` ao `startScan`.

`_FbpConnection._connectAndSetup()` (nova sequência):

| Passo | Fase emitida | Android | iOS |
|---|---|---|---|
| 1 | `connecting` | `device.connect(license: nonprofit, timeout: 12 s, mtu: null)` | idem |
| 2 | `securing` | `bondState.first`; se ≠ `bonded`: `createBond(timeout: securingTimeout.inSeconds)` (padrão 30 s, menor que o `PAIR_AUTH_TIMEOUT_MS` de 35 s do firmware, para o app desistir antes de o dongle derrubar). Se já `bonded`: segue (o *Security Request* do dongle dispara a cifração). | Nada a chamar; a fase só informa a UI de que um alerta pode aparecer (ele vem no passo 5). |
| 3 | `optimizingLink` | `requestMtu(mtu)` | — |
| 4 | `discovering` | `discoverServices()` | idem |
| 5 | `enablingNotify` | `setNotifyValue(true)` (CCCD exige cifra: serializa depois da cifração) | `setNotifyValue(true, timeout: securingTimeout.inSeconds)` — é aqui que o iOS costuma esperar o alerta de pareamento |
| 6 | `ready` | — | — |

Qualquer exceção nos passos 1–5: `_lastError = classifyLinkError(...)`,
`device.disconnect()` (melhor esforço), emite `failed`. Na queda depois de
`ready` (reconexão já existente): se a nova tentativa falhar com
`bondLost`, `bondLostIos`, `notAuthorized`, `remoteReset` ou
`pairingRejected`, emite `failed` **sem** nova tentativa (não martelar o
dongle); com `busy`, até 3 tentativas com espera de 2 s, 4 s, 8 s.

Classificação — função pura `classifyLinkError` em
`infra/ble/ble_link_error_classifier.dart` (sem importar o FBP: recebe
plataforma, etapa, código de queda e o tipo/código da exceção já extraídos
pelo adapter; testável):

| Plataforma | Sinal | `BleLinkError` |
|---|---|---|
| ambas | Qualquer exceção do FBP com o aparelho já desconectado (ex.: `deviceIsDisconnected` lançado por `createBond`/`setNotifyValue` quando o dongle derruba no meio) | classificar pelo `device.disconnectReason` (linhas abaixo), não pela exceção |
| Android | `createBond` falhou (`createBondFailed`: bond terminou em `none`; inclui o *Pairing Failed* que o dongle manda fora da janela quando já tem bond) | `pairingRejected` |
| Android | `createBond` estourou o prazo | `pairingTimeout` |
| Android | queda com `disconnectReason.code == 5` (dongle mandou `0x05`) | `notAuthorized` |
| Android | queda com código `6` (*PIN or Key Missing*) ou `61` (`0x3D`, *MIC failure*) | `bondLost` |
| Android | queda com código `9` (dongle mandou `0x09`) | `busy` |
| Android | queda com código `19` (`0x13`) durante `connecting`/`securing` | `remoteReset` (o dongle derrubou tudo pelo botão) |
| Android | erro GATT `5`/`15`/`137` numa operação | `notAuthorized` [verificar `137` nos aparelhos] |
| iOS | erro com código `14` (`peerRemovedPairingInformation`) | `bondLostIos` |
| iOS | erro com código `15` (`encryptionTimedOut`) | `pairingTimeout` |
| iOS | erro ATT `5`/`15` (*insufficient authentication/encryption*) | `notAuthorized` |
| iOS | queda (`peripheralDisconnected`, 7) durante `securing`/`discovering`/`enablingNotify` | `notAuthorized` (ambíguo; o VM desambigua com o anúncio, §5.6) |
| ambas | `timeout` do `connect` | `timeout` |
| ambas | demais | `other` |

Os códigos marcados devem ser confirmados nos cenários B4–B11/B15 da spec 03
e ajustados **só nesta tabela** (função pura com teste de tabela).

Limite do iOS: o `disconnectReason.code` no iOS é o `error.code` do
CoreBluetooth (`FlutterBluePlusPlugin.m`, `didDisconnectPeripheral`), sem o
motivo HCI. Por isso `remoteReset` (`0x13`) e `busy` (`0x09`) são
indistinguíveis de `notAuthorized` no iOS; a única desambiguação disponível
é a dica do anúncio (§5.6). No Android o código é o `status` cru do
`onConnectionStateChange`.

### 5.4 Data

`DongleDatasource` (o `SecureDongleDatasource` sai, §5.7):

```dart
bool get canManageBonds => _ble.canManageBonds;
Future<BleBondState> bondState(String deviceId) => _ble.bondState(deviceId);
Future<void> removeBond(String deviceId) => _ble.removeBond(deviceId);

/// Acompanha os anúncios de UM dongle (filtro por id), para saber quando a
/// janela de pareamento abre.
Stream<BleDevice?> watchDongle(String deviceId, {Duration timeout});

/// `connect` passa `securingTimeout: const Duration(seconds: 30)` (≤ prazo do firmware).
```

`DongleRepository` (interface no domínio) e `DongleRepositoryImpl`:

```dart
bool get canManageBonds;
Future<BleBondState> bondState(String deviceId);
/// Lança [BleBondFailure] se o SO não remover; [BleBondUnsupportedFailure] no iOS.
Future<void> removeBond(String deviceId);
/// Dica de pareamento do anúncio do dongle; `null` = dongle não visto ou sem o campo.
Stream<DonglePairingHint?> watchPairingHint(String deviceId, {Duration timeout});
/// Motivo da última falha da conexão ativa (repassa `BleConnection.lastError`).
BleLinkError? get lastLinkError;
```

`core/errors/ble_failure.dart` ganha `BleBondFailure` e
`BleBondUnsupportedFailure` (mesma forma das existentes: `message` + `cause`).

### 5.5 Application (use cases)

`application/connection/prepare_secure_connection_use_case.dart` (novo) —
combina `DongleRepository` + `LastDongleRepository` (dois repositories ⇒ use
case, `CLAUDE.md` §5.1):

```dart
enum SecureConnectionPlan { restore, pairNow, waitForButton }

class PrepareSecureConnectionUseCase {
  PrepareSecureConnectionUseCase(this._dongle, this._lastDongle);
  final DongleRepository _dongle;
  final LastDongleRepository _lastDongle;

  /// Decide como conectar a [deviceId]; [hint] vem do anúncio visto na busca.
  Future<SecureConnectionPlan> call({
    required String deviceId,
    DonglePairingHint? hint,
  });
}
```

| `canManageBonds` | `bondState` | `LastDongle.id == deviceId` | `hint.windowOpen` | Plano |
|---|---|---|---|---|
| sim (Android) | `bonded` | qualquer | qualquer | `restore` |
| sim | `none`/`bonding` | qualquer | `true` | `pairNow` |
| sim | `none`/`bonding` | qualquer | `false`/sem dica | `waitForButton` |
| não (iOS) | `unknown` | sim | qualquer | `restore` |
| não | `unknown` | não | `true` | `pairNow` |
| não | `unknown` | não | `false`/sem dica | `waitForButton` (tela oferece "Já pareei este iPhone" ⇒ `restore`) |

**[Decisão] D16 — O anúncio nunca decide sozinho uma ação destrutiva.** Um
atacante pode clonar o anúncio (`hasBond=0`). Por isso o plano não remove
bond com base na dica: tenta restaurar e só remove/orienta depois de uma
falha **real** de cifração (`bondLost`, `bondLostIos`, `notAuthorized`,
`remoteReset`).

`application/connection/forget_dongle_use_case.dart` (alterar): passa a
devolver `ForgetDongleResult{bondRemoved, needsManualForget}`:

1. `last = await _lastDongle.load()`;
2. `await _dongle.disconnect()`;
3. se `last != null && _dongle.canManageBonds`: `removeBond(last.id)`
   (falha ⇒ `bondRemoved=false`, não interrompe);
   se `!canManageBonds`: `needsManualForget = last != null`;
4. `await _lastDongle.clear()`.

### 5.6 UI: `ConnectingViewModel` e telas

`ConnectingState` ganha:

```dart
final LinkSecurityStep security;   // enum abaixo
final BleLinkError? linkError;     // última falha classificada
final bool dongleSeen;             // em waitingButton: o anúncio do dongle está chegando
```

```dart
enum LinkSecurityStep {
  idle, planning, waitingButton, pairing, restoring, secured,
  rejected, windowExpired, removingBond, forgetOnIos, failed,
}
```

Máquina de estados (todas as transições; `C` = `connect(deviceId)` do
repository; "dica" = `watchPairingHint`):

| De | Evento | Condição | Para | Ação |
|---|---|---|---|---|
| `idle`/qualquer terminal | `connect(id, hint)` | — | `planning` | zera `phase`, `prep`, `linkError`; chama o use case |
| `planning` | plano `restore` | — | `restoring` | `C` |
| `planning` | plano `pairNow` | — | `pairing` | `C` |
| `planning` | plano `waitForButton` | — | `waitingButton` | assina a dica (timeout 120 s) |
| `waitingButton` | dica `windowOpen=true` | — | `pairing` | cancela a dica; `C` |
| `waitingButton` | dica recebida | `windowOpen=false` | `waitingButton` | `dongleSeen=true` |
| `waitingButton` | 120 s sem janela | — | `waitingButton` | `dongleSeen=false`; tela mostra "Tentar de novo" (re-assina) |
| `waitingButton` | `assumePaired()` ("Já pareei este iPhone") | `!canManageBonds` | `restoring` | cancela a dica; `C` |
| `waitingButton` | `cancel()` | — | `idle` | cancela a dica |
| `pairing`/`restoring` | fase `ready` | — | `secured` | `_rememberDevice()`; `_prepare()` (ELM327, como hoje) |
| `pairing` | `failed` com `pairingRejected`/`pairingTimeout`/`notAuthorized` | última dica `windowOpen=true` | `rejected` | — |
| `pairing` | `failed` com `pairingRejected`/`pairingTimeout`/`notAuthorized` | dica `windowOpen=false` ou sem dica | `windowExpired` | — |
| `restoring` | `failed` com `bondLost`/`notAuthorized`/`remoteReset` | `canManageBonds` | `removingBond` | `disconnect()` da conexão antiga (o `removeBond` com o ACL ativo derruba o enlace e o `_FbpConnection` tentaria reconectar sozinho); `removeBond(id)`; em seguida volta a `planning` (agora `bondState=none` ⇒ `pairNow` ou `waitForButton`) |
| `restoring` | `failed` com `bondLostIos` | `!canManageBonds` | `forgetOnIos` | apaga o `LastDongle` (o registro deixou de valer) |
| `restoring` | `failed` com `notAuthorized` | `!canManageBonds` | `waitingButton` | apaga o `LastDongle`; mensagem "o dongle não reconheceu este iPhone" |
| `removingBond` | `removeBond` falhou | — | `failed` | mensagem orientando Ajustes › Dispositivos conectados |
| `rejected` | `retry()` | — | `planning` | — |
| `windowExpired` | `retry()` | — | `waitingButton` | — |
| `forgetOnIos` | `confirmForgotten()` ("Já esqueci") | — | `waitingButton` | — |
| `pairing`/`restoring` | `failed` com `busy` | (o adapter já tentou 3×) | `failed` | — |
| `pairing`/`restoring` | `failed` com `timeout`/`other` | — | `failed` | — |
| `secured` | queda e reconexão falha com `bondLost`/`notAuthorized`/`remoteReset`/`bondLostIos` | — | `removingBond` (Android) ou `forgetOnIos` (iOS) | mesmo tratamento de `restoring`; o `ConnectionGuard` leva à `ConnectionLostScreen`, que mostra a variante "pareamento perdido" (abaixo) |

Navegação (a `ConnectingScreen` continua dona do desfecho):

| `security` | Rota |
|---|---|
| `planning`, `pairing`, `restoring`, `removingBond`, `secured` (até `prep == done`) | `/connecting` |
| `waitingButton`, `rejected`, `windowExpired`, `forgetOnIos` | `/pairing` (`pushReplacement`) |
| `secured` e `prep == done` | `/connected` (como hoje) |
| `failed` | `/connection-lost` (como hoje) |

Telas:

| Tela | Mudança |
|---|---|
| `ui/connection/view/pairing_screen.dart` (nova, rota `AppRoutes.pairing = '/pairing'`) | `ConsumerWidget` que renderiza conforme `security`, reutilizando `ConnectionStateView`, `StepList`, `AppButton`, `StatusBadge`, `SpinnerRing`. **waitingButton**: título "Coloque o dongle em modo de pareamento"; passos "Com o dongle ligado, segure o botão por 5 segundos", "Solte; o dongle fica pareável por 45 segundos", "O pareamento começa sozinho e o sistema pode pedir confirmação"; status "Procurando o dongle…" / "Dongle encontrado — aguardando o botão" (`dongleSeen`); ação secundária "Já pareei este iPhone" só quando `!canManageBonds`; "Cancelar". **windowExpired**: "O tempo para parear acabou" + "Segurar o botão de novo". **rejected**: "Pareamento recusado" + "Tentar de novo". **forgetOnIos**: "Este iPhone guarda um pareamento antigo"; passos "Abra Ajustes › Bluetooth", "Toque em (i) ao lado de TCCeltaDongle", "Toque em Esquecer este dispositivo", "Volte e segure o botão do dongle por 5 s"; primária "Abrir Ajustes" (`AppSettings.openAppSettings(type: AppSettingsType.bluetooth)`; [verificar] no iOS, que pode abrir os ajustes do app); secundária "Já esqueci". |
| `connecting_screen.dart` | `connectingStepsFor` ganha o passo "Pareando com segurança" (sublabel "· LESC") dirigido por `securing`; o listener navega para `/pairing` nos estados da tabela acima. |
| `connection_lost_screen.dart` | Se `linkError` ∈ {`bondLost`, `bondLostIos`, `notAuthorized`, `remoteReset`}: título "O dongle não reconhece mais este aparelho", descrição "O botão do dongle foi pressionado ou o pareamento foi apagado.", primária "Parear novamente" (chama `connectingViewModel.repair()` = transição de `restoring` acima e vai para `/connecting`). Senão, como hoje. |
| `scan_screen.dart` | `select` vai para `AppRoutes.connecting` (não mais `pskSetup`) passando a dica do anúncio; cada dongle com `windowOpen` ganha `StatusBadge` "Pronto para parear". |
| `scan_view_model.dart` | `ScanState` ganha `Map<String, DonglePairingHint> hints` preenchido a partir de `BleDevice.manufacturerData` (a tela só lê). |
| `connection_guard.dart` | `_flowRoutes` e `_adapterOffExcludedRoutes`: trocar `AppRoutes.pskSetup` por `AppRoutes.pairing`. |
| `ble_permissions_screen.dart` | Lógica igual (reconexão automática com `LastDongle` ⇒ `restoring`); corrigir o comentário que cita a PSK. |
| `ui/connection/actions/forget_dongle_action.dart` | Usa `ForgetDongleResult`: se `needsManualForget`, mostra diálogo com os passos de "Esquecer este dispositivo" no iOS. |
| `more_screen.dart` | Remover o card "Criptografia" se ainda apontar para a tela de PSK (§5.7). |

**[Decisão] D17 — Reaproveitar o `ConnectingViewModel` (sem VM novo).** O
pareamento é uma etapa do mesmo fluxo de conexão e o `ConnectionGuard` já
observa esse provider; um VM separado duplicaria o estado de fase. A tela
nova (`PairingScreen`) é só outra *view* do mesmo estado.

**[Decisão] D18 — Usuário não escolhe "parear" ou "conectar".** O plano é
automático (tabela §5.5); o usuário só vê instruções quando precisa agir
(botão, Ajustes).

### 5.7 Camada PSK legada e coordenação com a Fase 1

**[Decisão] A8 = opção A (remover, com tag), já executada na Fase 1**
(spec 07 §1 da Fase 1; branch `chore/remove-security`). Esta fase **não
remove nada**: só confere que o app que recebe já está sem PSK no caminho de
conexão. Os itens abaixo ficam como lista de conferência (se algum ainda
existir na base em que esta fase for aplicada, fazer como descrito):

1. `connection_providers.dart`: `dongleDatasourceProvider` volta a
   `DongleDatasource(ref.read(bleServiceProvider))`; remover import de
   `settings_providers.dart` se não for mais usado.
2. `ScanScreen.select` → `AppRoutes.connecting`.
3. Rotas `pskSetup`, `cameraPermissions`, `pskScanQr` removidas de
   `app_routes.dart`/`app_router.dart`.
4. Limpeza de segredo esquecido: na inicialização (`bootstrap`/`main`, junto
   do `SharedPreferences`), `prefs.remove('psk_hex')` e
   `prefs.remove('security_mode')` — uma PSK antiga não deve ficar no
   aparelho.
5. Coordenação com a tela de configurações da Fase 1 (T_lat, B_max, T_GPS):
   - a rota `AppRoutes.settings` **continua existindo**; ela pertence à Fase 1;
   - se ela ainda aponta para `PskSettingsScreen`, trocar pela tela da
     Fase 1 (se já existir) ou removê-la junto com o card do "Mais" até a
     Fase 1 recolocá-la;
   - se a Fase 1 pôs os parâmetros dentro de `SettingsService`, manter a
     classe e apagar só `getPsk/setPsk/clearPsk/getSecurityMode/setSecurityMode`
     e as chaves `psk_hex`/`security_mode`; senão, o arquivo sai inteiro.

Estado herdado da Fase 1 (spec 07 §1): a camada PSK foi **apagada** (não
existe a flag `LEGACY_PSK_TRANSPORT`). O histórico da implementação está na
tag `versao-custom-secure-ble` (a tag `legado/psk-app-crypto` prevista
originalmente **não é necessária**).

Destino do código (A8 — decidido: **A, já feito**):

| Opção | O que fazer |
|---|---|
| **A — remover (adotada; feita na Fase 1)** | Tag de histórico: `versao-custom-secure-ble` (em vez de criar `legado/psk-app-crypto`); apagar `core/crypto/**`, `infra/ble/{encrypted_ble_connection,ble_security_handshake}.dart`, `infra/ble/README.md` (reescrever para o fluxo LESC), `data/datasources/secure_dongle_datasource.dart`, `domain/ble/security_mode.dart`, `ui/settings/view/{psk_settings_screen,qr_scan_screen,camera_permissions_screen}.dart`, `ui/settings/view_model/{qr_scan_view_model,camera_permissions_view_model}.dart`, providers de PSK/modo/câmera em `ui/settings/settings_providers.dart`, a pilha de permissão de câmera (`data/datasources/camera_permissions_datasource.dart`, `data/repositories/camera_permissions_repository_impl.dart`, `domain/repositories/camera_permissions_repository.dart`), os testes correspondentes (`test/core/crypto/*`, `test/infra/ble/{ble_security_handshake_test,encrypted_ble_connection_test}.dart`, `test/ui/settings/{psk_settings_screen_test,camera_permissions_screen_test}.dart`, `test/src/ui/settings/view_model/*`, `test/src/data/repositories/camera_permissions_repository_impl_test.dart`); dependências `mobile_scanner` e `pointycastle` (se nenhuma outra fase as usar: `grep -rn "pointycastle\|mobile_scanner" lib`); `CAMERA` e `uses-feature` de câmera no `AndroidManifest.xml`; `NSCameraUsageDescription` no `Info.plist`. |
| B — isolar como legado (descartada) | Mover para `lib/src/legacy/app_layer_crypto/` (e testes para `test/legacy/`), sem provider, rota ou import a partir de `lib/src/` ativo; manter dependências. |

Comentários a corrigir em qualquer opção: `ui/telemetry/view_model/panel_view_model.dart`
(cita `SecurityModeNotifier`), `forget_dongle_use_case.dart` (cita PSK),
`ble_permissions_screen.dart`.

## 6. Plano arquivo por arquivo

| Arquivo | Ação |
|---|---|
| `lib/src/domain/ble/ble_bond_state.dart` | novo (§5.1) |
| `lib/src/domain/ble/ble_link_error.dart` | novo (§5.1) |
| `lib/src/domain/ble/dongle_pairing_hint.dart` | novo (§5.1) |
| `lib/src/domain/ble/ble_device.dart` | `manufacturerData` (§5.1) |
| `lib/src/domain/ble/ble_connection.dart` | `securing`, `lastError` (§5.1) |
| `lib/src/services/ble/ble_service.dart` | §5.2 |
| `lib/src/infra/ble/ble_plus.dart` | §5.3 |
| `lib/src/infra/ble/ble_link_error_classifier.dart` | novo (§5.3) |
| `lib/src/data/datasources/dongle_datasource.dart` | §5.4 |
| `lib/src/domain/repositories/dongle_repository.dart` | §5.4 |
| `lib/src/data/repositories/dongle_repository_impl.dart` | §5.4 |
| `lib/src/core/errors/ble_failure.dart` | `BleBondFailure`, `BleBondUnsupportedFailure` |
| `lib/src/application/connection/prepare_secure_connection_use_case.dart` | novo (§5.5) |
| `lib/src/application/connection/forget_dongle_use_case.dart` | §5.5 |
| `lib/src/ui/connection/connection_providers.dart` | datasource sem PSK; `prepareSecureConnectionUseCaseProvider` |
| `lib/src/ui/connection/view_model/connecting_view_model.dart` | §5.6 (`connect(deviceId, {hint})`, `retry()`, `assumePaired()`, `confirmForgotten()`, `repair()`, `cancel()`) |
| `lib/src/ui/connection/view_model/scan_view_model.dart` | `hints` |
| `lib/src/ui/connection/view/pairing_screen.dart` | novo |
| `lib/src/ui/connection/view/{connecting,connection_lost,scan,ble_permissions}_screen.dart` | §5.6 |
| `lib/src/ui/connection/widgets/connection_guard.dart` | rotas |
| `lib/src/ui/connection/actions/forget_dongle_action.dart` | diálogo iOS |
| `lib/src/ui/more/view/more_screen.dart` | §5.7 (sem o card "Criptografia"; **sem** card "Segurança do enlace", A9) |
| `lib/src/router/app_routes.dart`, `app_router.dart` | `pairing`; remover rotas PSK |
| `lib/main.dart` (ou o *bootstrap* atual) | limpeza de `psk_hex`/`security_mode` |
| Camada PSK | §5.7: já removida na Fase 1 (A8 = A); só conferir |
| `pubspec.yaml`, `AndroidManifest.xml`, `Info.plist` | §5.7 (opção A): conferir que `mobile_scanner`, `pointycastle`, `CAMERA` e `NSCameraUsageDescription` já saíram |
| `mobile-app/CLAUDE.md` | Contrato: "o dongle exige pareamento LESC (bond) antes de qualquer escrita/notify"; nome anunciado `TCCeltaDongle`; remover menções à feature PSK e ao par `CameraPermissions*`; citar `securing` e `PairingScreen` na fatia `connection`. |
| `test/support/fake_ble_service.dart` | §7 |

## 7. Testes

`FakeBleService` ganha: `canManageBonds` (padrão `true`), `bondStates`
(`Map<String, BleBondState>`), `removeBondCalls` (lista), `removeBondError`,
`linkError` (o `FakeBleConnection` termina em `failed` com esse
`lastError`), `hintFrames` (quadros de `BleDevice` com `manufacturerData`
para `scan(withRemoteIds: …)`), e as fases padrão passam a incluir
`securing`. Helper `manufacturerFor({bool windowOpen, bool hasBond})`.

| Arquivo | Casos |
|---|---|
| `test/domain/ble/dongle_pairing_hint_test.dart` | `FF FF 01 01` ⇒ janela; `01 02` ⇒ bond; `01 03` ⇒ ambos; sem `0xFFFF` ⇒ `null`; versão `02` ⇒ `null`; 1 byte ⇒ `null`. |
| `test/infra/ble/ble_link_error_classifier_test.dart` | Uma linha por linha da tabela §5.3. |
| `test/application/connection/prepare_secure_connection_use_case_test.dart` | Uma linha por linha da tabela §5.5. |
| `test/application/connection/forget_dongle_use_case_test.dart` | Android remove bond; falha no `removeBond` não impede limpar o registro; iOS devolve `needsManualForget`. |
| `test/data/repositories/dongle_repository_impl_test.dart` | `watchPairingHint` mapeia `manufacturerData`; `removeBond` erro ⇒ `BleBondFailure`; iOS ⇒ `BleBondUnsupportedFailure`; `lastLinkError` repassa. |
| `test/ui/connection/connecting_view_model_test.dart` | (sem PSK) `restore` ⇒ `ready`; `pairNow` ⇒ fases com `securing` ⇒ `ready` e `LastDongle` salvo; `waitForButton` ⇒ quadros `[janela 0, janela 0, janela 1]` ⇒ `pairing` ⇒ `ready`; Android `bondLost` ⇒ `removeBond` chamado ⇒ `planning` ⇒ `waitingButton`; iOS `bondLostIos` ⇒ `forgetOnIos` e registro apagado; `pairingRejected` com janela aberta ⇒ `rejected`; `notAuthorized` com janela fechada ⇒ `windowExpired`; `assumePaired` ⇒ `restoring`; 120 s sem janela (fake async) ⇒ `dongleSeen=false`. |
| `test/ui/connection/pairing_screen_test.dart` | Renderiza cada estado; "Já pareei este iPhone" só com `canManageBonds=false`; "Já esqueci" leva a `waitingButton`. |
| `test/ui/connection/connection_lost_screen_test.dart` | Variante "pareamento perdido" com `linkError=bondLost`; padrão com `timeout`. |
| `test/ui/connection/scan_screen_test.dart` | Badge "Pronto para parear"; tocar vai para `/connecting` (não `/psk-setup`). |
| `test/ui/connection/widgets/connection_guard_test.dart` | `/pairing` não redireciona para "Conexão perdida". |

## 8. Passos ordenados com checkpoints

Em todo checkpoint: `fvm dart format .`, `fvm flutter analyze` ("No issues
found!"), `fvm flutter test`.

1. **Domínio + port + fake** (sem comportamento novo no adapter: implementar
   os métodos novos com o mínimo). *Checkpoint:* suíte atual verde.
2. **Tirar a PSK do caminho de conexão** (§5.7 itens 1–4) e ajustar
   `connecting_view_model_test.dart`. *Checkpoint:* app conecta a um dongle
   da Fase 2 já pareado manualmente (Ajustes do SO).
3. **Adapter** (§5.3) + classificador + teste de tabela. *Checkpoint:*
   bancada B4 (Android) chega a `ready` sem a tela de PSK.
4. **Data + use cases** + testes. *Checkpoint:* testes §7 dessas camadas.
5. **VM + telas + rotas + guard** + testes. *Checkpoint:* bancada B3→B4,
   B5, B8, B10, B15 (Android).
6. **iOS**: B9, B11 e o fluxo "Já pareei este iPhone". *Checkpoint:*
   tabela §5.3 confirmada; ajustes só no classificador.
7. **Conferir que a camada legada saiu** (A8 = A, feita na Fase 1) e
   manifest/plist/pubspec. *Checkpoint:* `grep -rn "pointycastle\|mobile_scanner\|psk" lib pubspec.yaml`
   vazio; `fvm flutter pub get`, build Android e iOS.
8. **Documentação** (`mobile-app/CLAUDE.md`, `infra/ble/README.md`).

## 9. Critérios de aceite

1. Do zero (sem bond), com a janela aberta: tocar no dongle na busca leva a
   "Conectado" passando por "Pareando com segurança", com um único pedido do
   SO, em Android e iOS.
2. Com a janela fechada: a tela pede para segurar o botão; ao abrir a janela,
   o pareamento começa sozinho em ≤ 2 s após o anúncio mudar.
3. Reabrir o app com bond existente reconecta sem diálogo e sem passar pela
   busca.
4. Botão apertado no dongle com o app conectado: Android remove o bond e
   pareia de novo sem o usuário ir aos Ajustes (janela aberta); iOS mostra
   as instruções de "Esquecer este dispositivo".
5. Cancelar o diálogo do SO leva a "Pareamento recusado"; perder a janela
   leva a "O tempo para parear acabou".
6. Nenhuma referência ativa à PSK no caminho de conexão; nenhuma PSK gravada
   no `SharedPreferences` após a primeira execução.
7. `format`, `analyze` e `test` verdes.

## 10. Riscos

| # | Risco | Mitigação |
|---|---|---|
| R14 | Códigos de erro diferentes por fabricante/versão do Android. | Classificador puro e tabelado; ajustar após a bancada; `other` cai em "Conexão perdida", que nunca é destrutivo. |
| R15 | `removeBond` por reflexão bloqueado em Androids novos. | Falha vira `failed` com instrução manual (Ajustes › Dispositivos conectados › Esquecer). |
| R16 | iOS sem bond guardado mas com registro do app (ex.: usuário esqueceu nos Ajustes) tenta `restore` e o iOS mostra um alerta de pareamento fora da janela. | O dongle recusa (`0x05`) ⇒ `notAuthorized` ⇒ `waitingButton` com mensagem clara. |
| R17 | Clone do anúncio do dongle induz o app a mostrar "pronto para parear" e o celular pareia com o **dongle falso** (MITM de Just Works dentro ou fora da janela do dongle real). | D16: a dica nunca dispara ação destrutiva. Limitação inerente a Just Works (A1.3); a autenticidade dos dados medidos continua garantida pela Fase 1 (o clone não tem `K_rec`), e o servidor rejeita blocos forjados. |
| R18 | Conflito de merge com a Fase 1 em `ble_plus.dart`, `BleConnectionPhase`, `FakeBleService`, telas de configurações. | Executar depois da Fase 1 (ordem do `.spec/README.md`); mudanças desta spec são aditivas nessas peças. |
| R19 | Diálogo de pareamento duplicado no Android. | O dongle não manda *Security Request* a quem não tem bond (spec 01 D21) e o app chama `createBond()` logo após o `connect` (recomendação do README do FBP); o plugin não repete o pedido se já estiver em `bonding`. |

## 11. Decisões e pontos em aberto

- **[Decisão] D16** — Dados do anúncio são só dica; ações destrutivas exigem
  falha real (§5.5).
- **[Decisão] D17** — Pareamento dentro do `ConnectingViewModel` (§5.6).
- **[Decisão] D18** — Plano automático, sem escolha do usuário (§5.6).
- **[Decisão] D19** — `createBond()` explícito no Android logo após conectar;
  no iOS nada (o SO conduz).
- **[Decisão] D20** — Reconexão não insiste após erro de segurança (evita
  martelar o dongle e acumular `CONN_REJECT`); só `busy` tem novas
  tentativas.
- **[Decisão] A8** — Destino da camada PSK: **A (remover com tag)**, **já
  feito na Fase 1**. O firmware não tem a contraparte, então o código não
  serve para medir nada; a comparação para o TCC continua em
  `docs/seguranca-ble-modelagem-ameacas.md` e a tag `versao-custom-secure-ble`
  preserva a implementação para consulta.
- **[Decisão] A9** — **Não criar** o card "Segurança do enlace" no "Mais"
  nesta fase (o status de conexão já aparece nas telas de conexão).
