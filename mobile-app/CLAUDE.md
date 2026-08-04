# CLAUDE.md

Este arquivo fornece orientações ao Claude Code (claude.ai/code) ao trabalhar com o código deste repositório.

## Contexto do projeto

`tccelta_mobile` (nome de exibição "OBD2 Cockpit") é a metade em Flutter do **tccelta**, um trabalho de conclusão de curso (TCC) de graduação em engenharia de computação da USP Poli. O sistema é um dongle OBD-II baseado em ESP32 que lê dados do veículo em tempo real pelo barramento CAN; este app é o companheiro que conversa com o dongle.

O monorepo mais amplo fica um nível acima, em `../`, e contém:
- `../esp32-firmware/` — o firmware do dongle (Arduino + FreeRTOS, PlatformIO). Em camadas: CAN → OBD-II → ELM327 → BLE.
- `../docs/` — documentação de protocolo e arquitetura. `can-e-obd2.md` (quadros CAN, serviços/PIDs OBD-II, ISO-TP) e `rtos-freertos.md` (modelo de concorrência do firmware) são o pano de fundo relevante para a integração app↔dongle.

O código é em português do Brasil (comentários, identificadores do domínio automotivo). Mantenha novos comentários/strings em pt-BR para combinar.

## Contrato de comunicação com o firmware

O app se comunica com o dongle via **BLE usando o Nordic UART Service**. Este é o contrato que o app deve implementar (definido em `../esp32-firmware/src/connectivity/ble/BleConnectivity.h`):

- O dispositivo anuncia-se como `OBD2Dongle`.
- Service UUID `6E400001-B5A3-F393-E0A9-E50E24DCCA9E`.
- **RX** `6E400002-…` (WRITE) — o app escreve aqui **comandos de texto ELM327**, ex.: `010C\r` (ler PID 0x0C = RPM do motor).
- **TX** `6E400003-…` (NOTIFY) — o dongle envia **respostas de texto ELM327**, ex.: `41 0C 17 70\r>`.

Os payloads são texto ASCII sobre BLE — não JSON/protobuf. A fila de comandos do firmware tem profundidade finita e **descarta comandos sob back-pressure**, então o app precisa lidar com timeouts/respostas ausentes de forma graciosa. Criptografia/autenticação (`SecureBleConnectivity`) está apenas esboçada (stub) no firmware hoje. Para testes sem hardware, o firmware pode rodar seu perfil PlatformIO `env:mock`.


## Comandos

Rodar a partir de `mobile-app/`:

- `flutter pub get` — baixar dependências
- `flutter run` — iniciar em um dispositivo/emulador conectado (`--release` para release)
- `flutter analyze` — lint (estrito; ver abaixo)
- `flutter test` — rodar todos os widget tests
- `flutter test test/widget_test.dart --plain-name "painel mostra os valores de exemplo dos PIDs"` — rodar um único teste pelo nome
- `dart run build_runner build --delete-conflicting-outputs` — regerar código após editar models freezed/json_serializable (ainda não existe nenhum, mas a toolchain está montada)

## Arquitetura

Em camadas, sob `lib/src/`, com `main.dart` como bootstrap fino. `main.dart` já monta `ProviderScope` + `MaterialApp.router` (go_router via `src/router/app_router.dart`) e entra pelo fluxo de conexão; um `ConnectionGuard` de escopo global é montado no `builder` do `MaterialApp`. A rota `/painel` abre a `PainelScreen` (feature `telemetry`) — o painel de telemetria OBD-II ao vivo.

- `src/core/theme/` — **design tokens**. Importar via o barrel `theme.dart`. Material 3 dark-first (`app_theme.dart`) montado a partir de `app_colors.dart`, `app_typography.dart` (Space Grotesk para UI, JetBrains Mono com algarismos tabulares para dados — carregado em runtime via `google_fonts`), `app_spacing.dart` (espaçamento + raios) e `app_effects.dart` (sombras, brilhos "live" ciano, durações de movimento com uma ponte de acessibilidade `MotionX` que respeita `disableAnimations`).
- `src/ui/core/widgets/` — **átomos do design system** (a biblioteca de componentes). Importar via o barrel `widgets.dart`. Inclui `AppButton`, `AppTabBar`, `StatusBadge`, `SensorRow`, `StatCard`, `StatGraphCard`, e o característico `Gauge` (ring/arc270/arc180, CustomPainter em `gauge/gauge_painter.dart`) e `Sparkline`.
- `src/ui/core/icons/` — sistema de ícones SVG: widget `AppIcon` + enum `AppIconData` (`app_icons.dart`), apoiado em 16 SVGs em `assets/icons/` recoloridos em runtime via `flutter_svg`.
- `src/ui/telemetry/` — feature **telemetry** (Fase 2 ELM327): lê os PIDs OBD-II sobre a `BleConnection` viva e exibe cada leitura em `StatCard.value` na `PainelScreen` (`/painel`). Camadas: `data/datasources/{elm327_client,obd2_datasource}.dart` (interações byte-a-byte + parsing) → `data/repositories/obd2_repository_impl.dart` (decodifica via `domain/obd2/obd2_pid.dart` e mapeia erro → `ObdCommandFailure`) → `view_model/telemetry_view_model.dart` (polling contínuo) → `view/painel_screen.dart`.

Ao adicionar elementos visuais, reutilize os tokens e átomos existentes em vez de fixar cores, espaçamentos ou estilos de texto no código — o design system é a única fonte da verdade para esses valores.

## Component-first: sempre fatore padrões reutilizáveis

Sempre que você construir qualquer coisa — um elemento de UI, um layout, um trecho de lógica de formatação/conversão, um padrão de estado — **primeiro procure o padrão** e decida se ele deve ser um componente reutilizável em vez de código inline. Isto é uma regra rígida, não um "seria bom ter":

- **Identifique o padrão antes de escrever.** Se um bloco visual, comportamento ou trecho de lógica pode plausivelmente aparecer mais de uma vez, trate-o como candidato a componente. Se você se pegar copiando e colando ou escrevendo algo estruturalmente parecido com código existente, pare e extraia.
- **Verifique primeiro se já existe um átomo.** Antes de criar qualquer coisa nova, busque em `src/ui/core/widgets/` (barrel `widgets.dart`) e nos tokens de tema. Reutilize ou estenda o que já existe em vez de duplicar. Se um átomo existente está *quase* certo, generalize-o (adicione um parâmetro/variante) em vez de bifurcar uma quase-cópia.
- **Coloque no lugar certo.** Átomos de UI reutilizáveis e agnósticos de domínio vão em `src/ui/core/widgets/` e são exportados pelo barrel `widgets.dart`. Constantes de design (cores, espaçamento, raios, tipografia, efeitos, movimento) vão em `src/core/theme/`, nunca fixas no ponto de uso. Telas de feature compõem esses átomos; não devem redefini-los.
- **Mantenha os componentes autossuficientes e configuráveis.** Conduza a variação por parâmetros/enums (como o `Gauge` faz com ring/arc270/arc180), exponha defaults sensatos e evite vazar premissas específicas de feature para um átomo compartilhado.
- **Na dúvida, extraia.** Prefira um widget/helper pequeno e bem nomeado a um grande bloco inline. Uma tela de feature deve se ler como uma composição de componentes nomeados, não como um paredão de código de layout.

O objetivo: o código permanece uma camada fina de composição de features sobre uma rica biblioteca de componentes e tokens que é fonte única da verdade.

## Estrutura de pastas
 
Mapa 1:1 com o projeto React Native do time:
 
```
lib/
  main.dart                  # delega para bootstrap()
  bootstrap.dart             # runApp(ProviderScope(child: App())) + init de services
  src/
    core/                    # infra transversal (sem regra de negócio de feature)
      config/                # AppEnvironment, flavors
      network/               # dio_client.dart + interceptors (auth, logging, retry)
      errors/                # Failure, AppException
      router/                # app_router.dart (go_router)
      theme/                 # ThemeData + design tokens
      utils/ extensions/
    domain/                  # CAMADA DOMAIN — só models/entities. PURA: zero deps externas.
      models/                #   importável por data, application e ui. Nada de dio/flutter/DTO aqui.
    application/             # CAMADA APPLICATION — use cases (regra de negócio / orquestração)
      <modulo>/              #   ex: checkout/place_order_use_case.dart
                             #   PODE depender de repositories (data) e de models (domain).
                             #   Criar SÓ quando necessário (ver §5.1) — nada de use case passthrough.
    data/                    # CAMADA DATA — obtenção de dados (reutilizável entre módulos)
      datasources/           # wrap de endpoints/plugins  (o guia oficial chama isto de "services")
      dtos/                  # modelos de transporte (freezed + json_serializable)
      repositories/          # SOURCE OF TRUTH; mapeiam DTO -> domain model; cache/erros/retry
                             #   único que fala com datasource. NÃO conhece outros repositories.
    services/                # CAMADA SERVICES — ports / side-effects / integrações externas (transversal)
      ble/                   #   ex: ble_service.dart -> PORT abstrato (BLE puro, sem "dongle")
      analytics_service.dart #   (analytics, notificações, storage, etc.) — NÃO confundir com use case
      notification_service.dart
    infra/                   # CAMADA INFRA — ADAPTERS concretos que implementam os ports de services/
      ble/                   #   ex: ble_plus.dart -> adapter sobre flutter_blue_plus (única camada
                             #   que conhece a lib); trocar de lib = reescrever aqui + trocar 1 provider
    ui/                      # CAMADA UI — organizada por MÓDULO (feature-first)
      <modulo>/
        view/                # <modulo>_screen.dart  -> View "burra" (só observa + renderiza)
        view_model/          # <modulo>_view_model.dart -> ViewModel (chama use case OU repository)
        widgets/             # componentes ESPECÍFICOS do módulo
      core/
        widgets/             # COMPONENTES GENÉRICOS (a "página de genéricos" do time)
        theme/
test/                        # espelha lib/src/ (mesma árvore)
```
 
## 5. Vertical slice de validação (feature `connection`)
 
A fatia vertical que **fecha a arquitetura ponta a ponta já existe e está implementada**: é a feature `connection` (fluxo de conexão BLE com o dongle). Ela é a referência viva das camadas — copie o padrão dela ao criar novas features. Fluxo: `infra` (adapter da lib) → `services` (port BLE) → `data` (datasource → repository, mapeia erro → `Failure`) → `ui` (view_model `Notifier` → view `Consumer` que só renderiza). Difere da slice CRUD/HTTP clássica por ser transporte BLE de streams e texto (ver Notas ao fim).
 
Peças da slice (todas já no código):
- `domain/ble/{ble_device,ble_adapter_state,ble_connection}.dart` — modelos de domínio PUROS (só `meta`, `@immutable`). **Sem freezed/json_serializable e sem DTO**: o transporte é texto ASCII sobre BLE, não JSON, então não há camada de DTO/`fromJson` aqui.
- `domain/repositories/{dongle,permissions}_repository.dart` — interfaces no `domain`.
- `services/ble/ble_service.dart` — **port** BLE abstrato (BLE puro, sem "dongle").
- `infra/ble/ble_plus.dart` — **adapter** concreto sobre `flutter_blue_plus`; única camada que conhece a lib. Trocar de lib BLE = reescrever só aqui + trocar 1 provider.
- `data/datasources/{dongle,permissions}_datasource.dart` — envolvem o port/plugins.
- `data/repositories/{dongle,permissions}_repository_impl.dart` — impls; *source of truth* da conexão/permissões; mapeiam erro cru → `Failure`/`BleFailure` (`src/core/errors/`).
- `ui/connection/view_model/*.dart` — ViewModels `Notifier<Estado>`: `ScanViewModel` (estado `ScanState` imutável + `copyWith`), `ConnectingViewModel` (fase `BleConnectionPhase`), `PermissionsViewModel` (fase `PermissionFlowState`). **Toda a lógica vive aqui.**
- `ui/connection/view/*.dart` — 6 telas `ConsumerWidget`/`ConsumerStatefulWidget` "burras" (só `ref.watch` + render).
- `ui/connection/widgets/*.dart` — componentes específicos do módulo (`ConnectionGuard`, `ConnectionBackground`, `ConnectionStateView`).
- `ui/connection/connection_providers.dart` — toda a DI via Riverpod (port → adapter, datasource → repository → view_model).
- Rotas registradas no `go_router` (`src/router/app_router.dart`, `src/router/app_routes.dart`).

> Notas (diferenças vs a slice CRUD/HTTP clássica):
> - **Sem DTO/freezed/json**: o transporte é texto ASCII sobre BLE, não JSON. `freezed` + `json_serializable` seguem sendo o padrão para models serializáveis QUANDO houver backend HTTP (via `dio`).
> - **`Notifier`, não `AsyncNotifier`**: o fluxo é orientado a streams/fases (scan contínuo, fases de conexão), não a um único fetch; o estado é síncrono e observável.
> - **Par `services` (port) + `infra` (adapter)** entre `ui`/`data` e a lib, para isolar o `flutter_blue_plus`. Numa slice HTTP o próprio `dio` já cumpre esse papel, então esse par não é necessário.
> - **Sem use case (`application/`)**: os view_models falam direto com os repositories — de propósito (ver §5.1).
 
## 5.1 Camada `application` (use cases) — quando e como
 
Regra prática: comece com `view_model → repository` — a camada `repository` **sempre** existe (é a *source of truth* e a única que fala com o datasource); o que é opcional é a camada `application/` (use cases) acima dela. Extraia um use case em `application/` **apenas** quando ocorrer um destes:
- a lógica combina dados de **mais de um repository** (repositories não se conhecem, então a orquestração precisa subir);
- a mesma operação de negócio é **reusada por múltiplos view_models**;
- o `view_model` está inchando de orquestração.
Não criar use case "passthrough" (uma linha que só repassa pro repository) — isso é boilerplate sem ganho. **Atenção:** esta regra do passthrough vale APENAS para a camada `application/` (use cases). Ela NÃO autoriza pular o repository: o `view_model` fala com o repository, nunca com o datasource, mesmo que o repository pareça um simples repasse (datasource → repository → view_model é sempre a cadeia mínima).
 
Na feature `connection` isto **ainda não ocorre**: cada view_model fala direto com um repository (`ScanViewModel`/`ConnectingViewModel` → `DongleRepository`; `PermissionsViewModel` → `PermissionsRepository`), e está correto — nenhum orquestra dois repositories nem repete regra. Portanto `connection` hoje NÃO tem camada `application/`.

Onde um use case entraria neste domínio: a sequência *checar permissão → confirmar adaptador ligado → iniciar conexão* hoje está dividida entre `PermissionsScreen`, `ConnectionGuard` e os view_models. Se ela precisar ser reusada por mais de um view_model (ou inchar), sobe para `application/connection/prepare_connection_use_case.dart`, combinando `PermissionsRepository` + `DongleRepository`:
 
```dart
// application/connection/prepare_connection_use_case.dart
class PrepareConnectionUseCase {
  PrepareConnectionUseCase(this._permissions, this._dongle);
  final PermissionsRepository _permissions; // orquestra repositories,
  final DongleRepository _dongle;            // nunca datasources
 
  Future<void> call(String deviceId) async {
    if (!await _permissions.hasBluetoothPermission()) {
      throw const BluetoothPermissionFailure(); // regra vive aqui
    }
    await _dongle.connect(deviceId);             // combina dois repositories
  }
}
 
// provider (DI via Riverpod)
final prepareConnectionUseCaseProvider = Provider(
  (ref) => PrepareConnectionUseCase(
    ref.read(permissionsRepositoryProvider),
    ref.read(dongleRepositoryProvider),
  ),
);
```
 
## 6. Testes (definição mínima)
 
A costura de teste é o **port `BleService`**: um `FakeBleService` (`test/support/fake_ble_service.dart`) substitui a lib BLE, e as camadas reais rodam por cima. Cobertura existente (espelha `lib/src/`):
- `DongleRepositoryImpl` sobre um `DongleDatasource(FakeBleService)`: `scan` emite os dongles vistos, erro cru vira `BleScanFailure`, e `connect` progride até `ready` expondo a conexão. (`PermissionsRepositoryImpl` tem teste análogo, com `mocktail` disponível para mockar dependências.)
- ViewModels via `ProviderContainer` sobrescrevendo `bleServiceProvider` pelo `FakeBleService`: `ScanViewModel` (popula `devices`; toques em rajada coalescem sem reiniciar o scan), `ConnectingViewModel` (progressão de fases → `ready`/`failed`) e `PermissionsViewModel` (`checking → granted/denied`).
- 1 widget test da `ScanScreen` (`test/ui/connection/scan_screen_test.dart`) renderizando os estados de busca.

## Stack e convenções

- **Estado**: `flutter_riverpod` já em uso (feature `connection`: `Notifier`/`NotifierProvider`, `StreamProvider`, `ConsumerWidget`). `hooks_riverpod` + `flutter_hooks` também em uso, de forma **seletiva**. Convenção: **estado de negócio → Riverpod** (ViewModels/providers); **estado efêmero de UI e ciclo de vida/animação → hooks** (`useState`, `useEffect`, `useAnimationController`). Na prática: telas sem estado local ficam `ConsumerWidget`; telas com estado efêmero viram `HookConsumerWidget` (mantêm o `WidgetRef ref` — `ref.watch`/`listen` inalterados); átomos animados que não tocam em `ref` são `HookWidget`. Não trocar `ConsumerWidget` por `HookConsumerWidget` sem necessidade (churn). Padrões de ciclo de vida repetidos viram hooks compartilhados em `src/ui/core/hooks/` (barrel `hooks.dart`) — ex.: `useLoopController(duration)`, que encapsula um `AnimationController` em loop respeitando `context.reduceMotion` (usado por `RadarScanner`, `_BleSpinner`, `_SearchingLabel`).
- **Roteamento**: `go_router` já fiado em `src/router/app_router.dart` (rotas em `app_routes.dart`), montado via `MaterialApp.router` no `main.dart`.
- **BLE**: `flutter_blue_plus` (transporte), `permission_handler` (permissões de BT/localização) e `app_settings` (abrir ajustes do sistema). `shared_preferences` disponível para persistência local.
- **Rede**: `dio` (para qualquer backend futuro).
- **Models**: `freezed` + `json_serializable` são o padrão pretendido para models serializáveis.
- **Linting**: `analysis_options.yaml` inclui `very_good_analysis` (estrito). Os gerados `*.g.dart`/`*.freezed.dart` são excluídos, e `invalid_annotation_target` é rebaixado para permitir freezed + json_serializable. Espera-se que código novo passe limpo no `flutter analyze`.
- **Testes**: `flutter_test` para widget tests; `mocktail` está disponível para mocking.

## Notas de plataforma

- namespace/applicationId Android: `br.com.malcong.tccelta_mobile`; Gradle é Kotlin DSL (`android/app/build.gradle.kts`), Java/Kotlin 17. O release atualmente assina com a chave de debug (placeholder — precisa de uma config de assinatura real antes de publicar).
- Apenas os targets Android e iOS estão configurados.
