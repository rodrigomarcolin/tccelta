# 08 — App: configurações de sincronização (`T_lat`, `B_max`, `T_GPS`) e envio periódico de GPS

## Objetivo

1. Tela de configurações, acessível pela aba "Mais", para os parâmetros do
   envio com buffer (Tabela "Parâmetros do envio com buffer no app" do
   documento): `T_lat` (padrão 60 s, 10–300 s) e `B_max` (padrão 64, ≥ 2 × 32),
   e para o período de GPS `T_GPS`; persistidos em `shared_preferences`. A
   espera entre tentativas (exponencial de 1 s a 5 min) é **constante no
   código** (spec 07, `kRetryMin`/`kRetryMax`).
2. Envio da posição do celular ao dongle com `AT#GPS lat lon alt` (graus ×
   10⁷ e milímetros) pelo NUS a cada `T_GPS`, enquanto conectado.

## Escopo

**Dentro:** modelo, datasource, repository e view model das configurações;
tela e rota; plugin de localização, permissões Android/iOS; `LocationRepository`;
`reportGps` no `Obd2Repository`; `GpsReporter` (use case); testes.

**Fora:** uso de `T_lat`/`B_max` pelo coordenador (spec 07 — aqui só a fonte);
`T_lat` sugerido pelo servidor no handshake (spec 09 guarda o valor; aplicar
como *override* é decisão da Fase 3); localização em segundo plano.

## Estado atual (verificado no código)

| Peça | Onde | Hoje |
|---|---|---|
| Persistência de ajustes | `lib/src/services/settings/settings_service.dart` | `SettingsService` com `SharedPreferences.getInstance()` e chaves `psk_hex`, `security_mode`. |
| Persistência injetável | `lib/src/core/persistence/shared_preferences_provider.dart`, `lib/src/data/datasources/last_dongle_datasource.dart` | `sharedPreferencesProvider` (sobrescrito no `main`); datasources recebem `SharedPreferences` e só leem/escrevem valores crus. |
| Aba "Mais" | `lib/src/ui/more/view/more_screen.dart` | `CardButton`s "Criptografia" e "Esquecer dispositivo". |
| Átomos | `lib/src/ui/core/widgets/` | `NumberStepper` (rótulo, subtítulo, min/max/step), `SegmentedToggle`, `CardButton`, `StatusBadge`, `Callout`. |
| Rotas | `lib/src/router/app_routes.dart`, `app_router.dart` | `/more`, `/settings` (PSK) etc. |
| Comandos ELM327 | `lib/src/data/datasources/elm327_client.dart`, `lib/src/data/repositories/obd2_repository_impl.dart` | Um único `Elm327Client` por conexão, serializando comandos, criado dentro do `Obd2RepositoryImpl` e compartilhado por telemetria e DTC. |
| Firmware | `esp32-firmware/src/collector/AtHashParser.cpp`, `GpsGate.cpp` | `AT#GPS lat lon alt`: lat ∈ [−900 000 000, 900 000 000], lon ∈ [−1 800 000 000, 1 800 000 000], alt ∈ int32; responde `OK` (ou `?` se inválido). Aceita no máximo 1 posição/s (tolerância de 100 ms); descarta no modo diagnóstico (`GPS_DROP`). |
| Permissões | `android/app/src/main/AndroidManifest.xml`, `ios/Runner/Info.plist` | `ACCESS_FINE_LOCATION` com `android:maxSdkVersion="30"` (só para o scan BLE antigo); iOS sem texto de localização. |

## Design

### 1. Parâmetros

| Parâmetro | Chave (`shared_preferences`) | Tipo | Padrão | Faixa / passo | Efeito ao mudar |
|---|---|---|---|---|---|
| `T_lat` | `sync_t_lat_s` | int (s) | 60 | 10–300, passo 10 | Próxima avaliação da regra de envio (≤ 1 s). |
| `B_max` | `sync_b_max` | int (blocos) | 64 | 64–512, passo 32 | Imediato; reduzir não descarta blocos já no buffer, só para de pedir lotes até caber. |
| `T_GPS` | `gps_period_s` | int (s) | 1 | 1–60, passo 1 | Reinicia o temporizador do GPS. |
| GPS ligado | `gps_enabled` | bool | `true` | — | Liga/desliga o `GpsReporter`. |
| Espera entre tentativas | — (constante) | — | 1 s → 5 min, ×2 | — | `RetryBackoff` (spec 07). |

**[Decisão]** `B_max` mínimo 64 = 2 × 32 (o documento: "ao menos o dobro do
máximo de 32 blocos aceito por POST") e máximo 512 (~560 KB de RAM no pior
caso, perfil `asym`). **[Decisão]** `T_GPS` padrão 1 s: é a taxa
`f_G = 1 posição/s` usada nas contas da Investigação (Tabelas 11 e 14) e o
máximo que o `GpsGate` do firmware aceita.

Valores fora da faixa lidos do armazenamento (versões futuras, edição
manual) são **ajustados para a faixa** (`clamp`) e regravados.

### 2. Modelo e camadas

```dart
// lib/src/domain/sync/sync_settings.dart
@immutable
class SyncSettings {
  const SyncSettings({
    this.latency = const Duration(seconds: 60),
    this.bufferCapacity = 64,
    this.gpsPeriod = const Duration(seconds: 1),
    this.gpsEnabled = true,
  });
  static const Duration minLatency = Duration(seconds: 10), maxLatency = Duration(seconds: 300);
  static const int minBuffer = 64, maxBuffer = 512;
  static const Duration minGpsPeriod = Duration(seconds: 1), maxGpsPeriod = Duration(seconds: 60);
  final Duration latency;
  final int bufferCapacity;
  final Duration gpsPeriod;
  final bool gpsEnabled;
  SyncSettings clamped();
  SyncSettings copyWith({...});
}
```

| Camada | Arquivo | Responsabilidade |
|---|---|---|
| Datasource | `lib/src/data/datasources/sync_settings_datasource.dart` | `SyncSettingsDatasource(SharedPreferences)`: `int? readInt(key)`, `bool? readBool(key)`, `Future<void> writeInt/writeBool`. Não conhece o modelo (mesmo papel do `LastDongleDatasource`). Chaves do §1. |
| Interface | `lib/src/domain/repositories/sync_settings_repository.dart` | `SyncSettings get current; Stream<SyncSettings> watch(); Future<void> save(SyncSettings)`. |
| Impl | `lib/src/data/repositories/sync_settings_repository_impl.dart` | Monta o modelo com padrões e `clamped()`, grava, emite no `watch` (broadcast + valor atual na assinatura). Erro de escrita → `SettingsWriteFailure`. |
| View model | `lib/src/ui/settings/view_model/sync_settings_view_model.dart` | `Notifier<SyncSettingsState>` (`settings`, `gpsPermission`, `saving`); `setLatency`, `setBufferCapacity`, `setGpsPeriod`, `setGpsEnabled`, `requestGpsPermission`. Fala com `SyncSettingsRepository` e `LocationRepository`. |
| Tela | `lib/src/ui/settings/view/sync_settings_screen.dart` | §3. |

**[Decisão]** Persistência no estilo do `SettingsService` (chaves simples no
`shared_preferences`), mas com a instância injetada pelo
`sharedPreferencesProvider` e a cadeia datasource → repository → view model
exigida pelo `CLAUDE.md` (como `LastDongleDatasource`), o que permite testar
com `SharedPreferences.setMockInitialValues`.

### 3. Tela `SyncSettingsScreen` (rota `/sync-settings`)

`ConsumerWidget`, composição de átomos existentes:

| Seção | Conteúdo |
|---|---|
| "Envio ao servidor" | `NumberStepper` "Latência máxima (T_lat)" — sublabel "tempo máximo de um bloco no buffer · s", 10–300, passo 10; `NumberStepper` "Buffer (B_max)" — sublabel "blocos guardados no celular · mínimo 2 × 32", 64–512, passo 32; `Callout` informativo: "Novas tentativas: 1 s a 5 min (fixo)". |
| "Posição (GPS)" | `SegmentedToggle` Ligado/Desligado; `NumberStepper` "Período (T_GPS)" 1–60 s; `StatusBadge` da permissão (concedida / negada / negada para sempre) e `AppButton` "Permitir localização" (ou "Abrir ajustes" se negada para sempre, via `app_settings`). |
| "Estado" | Resumo do `SyncStatus` (spec 07): fase, `DevId`, `HEAD`, `ACK`, último lido, buffer `n/B_max`, em POST, *flags*, próxima tentativa, modo do servidor (`stub`/`bancada`/`https`). |
| "Depuração" (só `kDebugMode`) | "Forçar handshake"; "Esquecer estado do dongle no servidor de bancada" (spec 09). |

`MoreScreen`: novo `CardButton` "Sincronização" (ícone existente de
transferência/nuvem do `AppIconData`; se não houver, adicionar um SVG ao
conjunto do design system), subtítulo `T_lat 60 s · buffer 64 · GPS 1 s`,
`onTap: context.push(AppRoutes.syncSettings)`; o cartão de estado da spec 07
fica logo abaixo. `AppRoutes.syncSettings = '/sync-settings'` + `GoRoute`.

### 4. Localização — plugin e permissões

**[Decisão] `geolocator`** (Baseflow; fixar a versão estável corrente com
`fvm flutter pub add geolocator`): Android e iOS; fluxo de posições com
intervalo e precisão configuráveis (`AndroidSettings(intervalDuration, accuracy: high, distanceFilter: 0)`,
`AppleSettings(activityType: automotiveNavigation)`); entrega altitude e
precisão; API própria de permissão (`checkPermission`/`requestPermission`),
sem exigir Google Play Services (`forceLocationManager` como alternativa).
O pacote `location` foi descartado por manutenção mais irregular, e o
`permission_handler` (já presente) não fornece posições.

| Plataforma | Mudança |
|---|---|
| Android `AndroidManifest.xml` | Remover `android:maxSdkVersion="30"` de `ACCESS_FINE_LOCATION` (agora necessária em todas as versões) e adicionar `ACCESS_COARSE_LOCATION`. Sem `ACCESS_BACKGROUND_LOCATION`. |
| iOS `Info.plist` | `NSLocationWhenInUseUsageDescription` = "O app envia a posição do celular ao dongle para registrar onde cada trecho da viagem aconteceu." |

O app **só pede permissão a partir da UI** (botão da tela, §3); o
`GpsReporter` nunca dispara diálogo — sem permissão, fica parado com estado
`semPermissao`.

### 5. `LocationRepository` e `GpsFix`

```dart
// lib/src/domain/location/gps_fix.dart
@immutable
class GpsFix {
  const GpsFix({required this.latE7, required this.lonE7, required this.altMm, required this.accuracyM, required this.at});
  final int latE7;      // graus × 10⁷, [−900000000, 900000000]
  final int lonE7;      // graus × 10⁷, [−1800000000, 1800000000]
  final int altMm;      // mm, int32
  final double accuracyM;
  final Duration at;    // monotônico
  /// Texto do comando ELM327 (sem terminador).
  String toAtCommand() => 'AT#GPS $latE7 $lonE7 $altMm';
  static GpsFix fromDegrees({required double lat, required double lon, required double altM, required double accuracyM, required Duration at});
}

enum LocationPermissionState { granted, denied, deniedForever, serviceDisabled }

abstract interface class LocationRepository {
  Future<LocationPermissionState> permission();
  Future<LocationPermissionState> requestPermission();
  Stream<GpsFix> fixes({required Duration interval});
}
```

Conversão: `latE7 = (lat × 1e7).round()` (idem `lon`), `altMm = (altM × 1000).round()`,
todos com `clamp` às faixas do parser do firmware. Altitude: a do
`Position.altitude` (elipsoidal no Android, acima do nível do mar no iOS) —
registrado como limitação (o servidor não depende dela).

`LocationDatasource` (`lib/src/data/datasources/location_datasource.dart`)
envolve o `geolocator` (único arquivo do app que importa o plugin);
`LocationRepositoryImpl` converte `Position` → `GpsFix` e erros →
`LocationFailure`.

### 6. `reportGps` no `Obd2Repository`

`Obd2Repository` ganha:

```dart
/// Envia a posição do celular ao dongle (`AT#GPS`). Lança `ObdCommandFailure`
/// se não houver conexão pronta ou o dongle responder diferente de `OK`.
Future<void> reportGps(GpsFix fix);
```

Implementado no `Obd2RepositoryImpl` sobre o mesmo `Elm327Client`
(`command(fix.toAtCommand(), timeout: 2 s)`), via um novo
`CollectorCommandDatasource(Elm327Client)` criado junto do `Obd2Datasource`/
`DtcDatasource`. **[Decisão]** O comando AT# vai pelo repository que já é
dono do cliente ELM327: dois clientes na mesma `BleConnection` capturariam
as respostas um do outro (mesmo motivo pelo qual telemetria e DTC já
compartilham o repository).

### 7. `GpsReporter` (`lib/src/application/gps/gps_reporter.dart`)

Use case de orquestração (`DongleRepository` para a fase da conexão,
`LocationRepository`, `Obd2Repository`, `SyncSettingsRepository`).

```dart
class GpsReporter {
  GpsReporter({required DongleRepository dongle, required LocationRepository location,
               required Obd2Repository obd2, required SyncSettingsRepository settings,
               MonotonicClock clock = const SystemMonotonicClock()});
  Stream<GpsReportStatus> get status;   // parado | semPermissao | aguardandoFix | enviando(últimoEnvio) | erro(msg)
  void start();
  Future<void> dispose();
}
```

| Condição | Comportamento |
|---|---|
| Conexão `ready` + `gpsEnabled` + permissão concedida | Assina `fixes(interval: T_GPS)`; `Timer.periodic(T_GPS)`: se há *fix* com idade ≤ 2 × `T_GPS` e precisão ≤ 100 m e nenhum envio em curso → `obd2.reportGps(fix)`. |
| *Fix* velho ou impreciso | Não envia (o firmware não tem como saber; melhor ausência que posição errada). |
| Falha do comando (`?`, *timeout*) | Registra em `status`; tenta no próximo período (sem *backoff*: é só 1 comando/s). |
| `reconnecting`/`disconnected`/`failed` | Cancela fluxo e temporizador. |
| Mudança de `T_GPS`/`gpsEnabled` | Reinicia. |

Provider `gpsReporterProvider` (em `lib/src/ui/gps/gps_providers.dart`,
`keepAlive`, `start()` + `onDispose`), instanciado pelo `ConnectionGuard`
como o `syncCoordinatorProvider`.

**[Decisão]** Constantes `kGpsMaxAccuracyM = 100` e idade máxima `2 × T_GPS`
no próprio `GpsReporter` (não configuráveis na Fase 1).

## Plano arquivo por arquivo

| Ação | Arquivo | Mudança |
|---|---|---|
| Criar | `lib/src/domain/sync/sync_settings.dart` | §2. |
| Criar | `lib/src/data/datasources/sync_settings_datasource.dart`, `lib/src/domain/repositories/sync_settings_repository.dart`, `lib/src/data/repositories/sync_settings_repository_impl.dart` | §2. |
| Criar | `lib/src/ui/settings/view_model/sync_settings_view_model.dart`, `lib/src/ui/settings/view/sync_settings_screen.dart` | §2–3. |
| Alterar | `lib/src/ui/settings/settings_providers.dart` | Providers de datasource/repository/view model das configurações. |
| Alterar | `lib/src/router/app_routes.dart`, `app_router.dart` | `/sync-settings`. |
| Alterar | `lib/src/ui/more/view/more_screen.dart` | Cartão "Sincronização". |
| Criar | `lib/src/domain/location/gps_fix.dart`, `lib/src/domain/repositories/location_repository.dart`, `lib/src/data/datasources/location_datasource.dart`, `lib/src/data/repositories/location_repository_impl.dart`, `lib/src/core/errors/location_failure.dart` | §4–5. |
| Criar | `lib/src/data/datasources/collector_command_datasource.dart` | `sendGps(GpsFix)` sobre `Elm327Client`. |
| Alterar | `lib/src/domain/repositories/obd2_repository.dart`, `lib/src/data/repositories/obd2_repository_impl.dart` | `reportGps` (§6). |
| Criar | `lib/src/application/gps/gps_reporter.dart`, `lib/src/ui/gps/gps_providers.dart` | §7. |
| Alterar | `lib/src/ui/connection/widgets/connection_guard.dart` | `ref.watch(gpsReporterProvider)`. |
| Alterar | `pubspec.yaml` | `geolocator`. |
| Alterar | `android/app/src/main/AndroidManifest.xml`, `ios/Runner/Info.plist` | §4. |
| Alterar | `test/support/fake_obd2_repository.dart` | `reportGps` registrando as chamadas. |
| Criar | `test/support/fake_location_repository.dart` | Fluxo de *fixes* controlável. |

## Testes

| Arquivo | Casos |
|---|---|
| `test/domain/sync/sync_settings_test.dart` | Padrões; `clamped()` nas bordas (5 s → 10 s, 400 s → 300 s, 32 → 64, 1000 → 512, 0 s → 1 s). |
| `test/data/repositories/sync_settings_repository_impl_test.dart` | `setMockInitialValues` vazio → padrões; valores gravados relidos; valor fora da faixa ajustado e regravado; `watch` emite o atual e cada `save`. |
| `test/ui/settings/sync_settings_view_model_test.dart` | `ProviderContainer` com dublês: cada setter persiste; pedido de permissão atualiza o estado. |
| `test/ui/settings/sync_settings_screen_test.dart` | Renderiza os três steppers com valores; `+`/`−` respeitam passo e limites; botão de permissão aparece quando negada. |
| `test/domain/location/gps_fix_test.dart` | `fromDegrees(-23.5505200, -46.6333090, 760.5)` → `-235505200 -466333090 760500`; arredondamento; `clamp`; `toAtCommand()`. |
| `test/data/repositories/obd2_repository_impl_test.dart` (existente) | + `reportGps` escreve `AT#GPS …` no `ScriptedBleConnection` e aceita `OK\r>`; `?\r>` → `ObdCommandFailure`. |
| `test/application/gps/gps_reporter_test.dart` (`fake_async`) | Envia a cada `T_GPS` com *fix* recente; não envia com *fix* velho ou impreciso; para ao desconectar; reinicia ao mudar `T_GPS`; sem permissão não envia nem pede. |

Em bancada: com o app conectado ao dongle (`collector_mock_debug`), o log do
firmware mostra `CMD txt="AT#GPS …"`, `GPS lat=… lon=… alt=…` e `REC kind=gps`
a cada `T_GPS` (≥ 1 s), e nenhum `GPS_DROP reason=rate` com `T_GPS = 1 s`.

## Passos de implementação

1. Modelo + datasource + repository + testes. *Checkpoint:* testes de configurações verdes.
2. View model + tela + rota + cartão. *Checkpoint:* widget test verde; navegação manual Mais → Sincronização.
3. `geolocator`, permissões, `LocationRepository`. *Checkpoint:* em aparelho, a tela mostra a permissão e o fluxo de *fixes* no log.
4. `reportGps` + `GpsReporter`. *Checkpoint:* `gps_reporter_test` verde; em bancada, `REC kind=gps` no firmware.
5. `fvm dart format .`, `fvm flutter analyze`, `fvm flutter test`.

## Critérios de aceite

- `T_lat`, `B_max`, `T_GPS` e GPS ligado/desligado editáveis na aba Mais →
  Sincronização, com faixas e padrões do §1, persistidos e aplicados sem
  reiniciar o app; espera entre tentativas como constante.
- Com conexão pronta e permissão concedida, `AT#GPS lat lon alt` (graus × 10⁷,
  mm) sai pelo NUS a cada `T_GPS`, sem disputar respostas com a telemetria.
- Permissões de localização declaradas em Android e iOS; nenhum pedido de
  permissão fora da UI.
- Cadeia datasource → repository → view model respeitada; `geolocator` só no
  datasource de localização.

## Riscos e decisões em aberto

| Risco / ponto | Impacto | Recomendação |
|---|---|---|
| `T_GPS = 1 s` + telemetria no mesmo NUS | Leituras do painel um pouco mais lentas (~20–50 ms por segundo) | Aceitável; medir no `RESP dur` do firmware. Se incomodar, padrão 2 s. |
| Localização com app em segundo plano | GPS para quando o app vai para segundo plano | Fora do escopo; **[Aberto]** se o autor quiser rastrear com tela apagada, exige `ACCESS_BACKGROUND_LOCATION`/`UIBackgroundModes` e serviço em primeiro plano no Android. |
| Altitude elipsoidal × nível do mar | Diferenças de dezenas de metros entre plataformas | Registrar no TCC; o servidor não usa altitude para nada crítico. |
| **[Aberto]** `T_lat` sugerido pelo servidor (`t_lat_s` no handshake, Fase 3) | Qual vale: o local ou o do servidor | Recomendação: o do servidor vira teto (`min(local, servidor)`) quando a Fase 3 existir; na Fase 1 só é guardado no `HandshakeOutcome`. |
