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

> Nota: ainda não há pacote BLE no `pubspec.yaml` — a camada de conectividade não está implementada. O app hoje é o design system mais uma tela de showcase.

## Comandos

Rodar a partir de `mobile-app/`:

- `flutter pub get` — baixar dependências
- `flutter run` — iniciar em um dispositivo/emulador conectado (`--release` para release)
- `flutter analyze` — lint (estrito; ver abaixo)
- `flutter test` — rodar todos os widget tests
- `flutter test test/widget_test.dart --plain-name "app builds the design system showcase"` — rodar um único teste pelo nome
- `dart run build_runner build --delete-conflicting-outputs` — regerar código após editar models freezed/json_serializable (ainda não existe nenhum, mas a toolchain está montada)

## Arquitetura

Em camadas, sob `lib/src/`, com `main.dart` como um bootstrap fino. `main.dart` atualmente monta `ShowcaseScreen` diretamente; a fiação de ProviderScope/go_router e as camadas de feature data/domain/service estão adiadas (anotado em `main.dart`).

- `src/core/theme/` — **design tokens**. Importar via o barrel `theme.dart`. Material 3 dark-first (`app_theme.dart`) montado a partir de `app_colors.dart`, `app_typography.dart` (Space Grotesk para UI, JetBrains Mono com algarismos tabulares para dados — carregado em runtime via `google_fonts`), `app_spacing.dart` (espaçamento + raios) e `app_effects.dart` (sombras, brilhos "live" ciano, durações de movimento com uma ponte de acessibilidade `MotionX` que respeita `disableAnimations`).
- `src/ui/core/widgets/` — **átomos do design system** (a biblioteca de componentes). Importar via o barrel `widgets.dart`. Inclui `AppButton`, `AppTabBar`, `StatusBadge`, `SensorRow`, `StatCard`, `StatGraphCard`, e o característico `Gauge` (ring/arc270/arc180, CustomPainter em `gauge/gauge_painter.dart`) e `Sparkline`.
- `src/ui/core/icons/` — sistema de ícones SVG: widget `AppIcon` + enum `AppIconData` (`app_icons.dart`), apoiado em 16 SVGs em `assets/icons/` recoloridos em runtime via `flutter_svg`.
- `src/ui/showcase/view/showcase_screen.dart` — galeria de todos os tokens e átomos com dados de sensores simulados ao vivo. Apenas referência/onboarding — não é uma tela de feature.

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
    services/                # CAMADA SERVICES — side-effects / integrações externas (transversal)
      analytics_service.dart #   (analytics, notificações, storage, etc.) — NÃO confundir com use case
      notification_service.dart
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
 
## 5. Vertical slice de validação (feature `products`)
 
Implementar UMA feature ponta a ponta, como prova de que a arquitetura está fechada. Fluxo: `dio_client` → `ProductsDatasource` → `ProductsRepository` (DTO→domain) → `ProductsViewModel` (AsyncNotifier) → `ProductsScreen` (só renderiza `AsyncValue.when`).
 
Requisitos da slice:
- `domain/models/product.dart` — modelo de domínio (freezed).
- `data/dtos/product_dto.dart` — DTO com `fromJson` (freezed + json_serializable) e `toDomain()`.
- `data/datasources/products_datasource.dart` — usa `dio`, retorna `List<ProductDto>`.
- `data/repositories/products_repository.dart` — interface no `domain` + impl no `data`; expõe `Future<List<Product>>`; trata erros virando `Failure`.
- `ui/products/view_model/products_view_model.dart` — `AsyncNotifier<List<Product>>` com `build()` (fetch) e `refresh()`. **Toda a lógica vive aqui.**
- `ui/products/view/products_screen.dart` — `ConsumerWidget` que faz `ref.watch(...)` e trata `loading/error/data` via `AsyncValue.when`. Sem lógica.
- `ui/products/widgets/product_card.dart` — widget específico do módulo.
- `ui/core/widgets/app_button.dart` — exemplo de componente genérico.
- Providers conectando datasource → repository → view_model (DI via Riverpod).
- Rota `/products` registrada no `go_router`.
> Nota: a slice `products` é CRUD simples e o `view_model` chama o `repository` **direto** — de propósito, NÃO tem use case. Use cases só entram quando a regra justifica (ver §5.1).
 
## 5.1 Camada `application` (use cases) — quando e como
 
Regra prática: comece com `view_model → repository`. Extraia um use case em `application/` **apenas** quando ocorrer um destes:
- a lógica combina dados de **mais de um repository** (repositories não se conhecem, então a orquestração precisa subir);
- a mesma operação de negócio é **reusada por múltiplos view_models**;
- o `view_model` está inchando de orquestração.
Não criar use case "passthrough" (uma linha que só repassa pro repository) — isso é boilerplate sem ganho.
 
Segunda slice de validação (feature `checkout`) para exercitar a camada:
- `application/checkout/place_order_use_case.dart` — classe *callable* (`call()`) que depende de `CartRepository` + `PaymentRepository`, aplica a regra (ex.: carrinho vazio → `Failure`, aplica desconto) e retorna um domain model `Order`. **Depende de repositories concretos, NUNCA de datasource.**
- `ui/checkout/view_model/checkout_view_model.dart` — chama `placeOrderUseCase()`; não conhece carrinho, pagamento nem datasource.
- Provider do use case injetando os dois repositories.
Esqueleto de referência:
 
```dart
// application/checkout/place_order_use_case.dart
class PlaceOrderUseCase {
  PlaceOrderUseCase(this._cart, this._payments);
  final CartRepository _cart;         // orquestra repositories,
  final PaymentRepository _payments;  // nunca datasources
 
  Future<Order> call() async {
    final cart = await _cart.current();
    if (cart.isEmpty) throw const EmptyCartFailure();
    final total = cart.applyDiscounts();   // regra de negócio vive aqui
    return _payments.charge(total);        // combina dois repositories
  }
}
 
// provider (DI via Riverpod)
final placeOrderUseCaseProvider = Provider(
  (ref) => PlaceOrderUseCase(
    ref.read(cartRepositoryProvider),
    ref.read(paymentRepositoryProvider),
  ),
);
```
 
## 6. Testes (definição mínima)
 
- Teste unitário do `ProductsRepository` com `dio` mockado (`mocktail`), cobrindo sucesso e mapeamento de erro → `Failure`.
- Teste do `ProductsViewModel` com repository mockado, validando transição loading → data e o caso de erro (usar `ProviderContainer` para overrides).
- Teste unitário do `PlaceOrderUseCase` com os dois repositories mockados (`mocktail`): caminho feliz, carrinho vazio → `Failure`, e desconto aplicado corretamente.
- 1 widget test da `ProductsScreen` renderizando os 3 estados.

## Stack e convenções

- **Estado**: `flutter_riverpod` + `hooks_riverpod` + `flutter_hooks` (declarados; ainda não usados).
- **Roteamento**: `go_router` (declarado; ainda não fiado).
- **Rede**: `dio` (para qualquer backend futuro).
- **Models**: `freezed` + `json_serializable` são o padrão pretendido para models serializáveis.
- **Linting**: `analysis_options.yaml` inclui `very_good_analysis` (estrito). Os gerados `*.g.dart`/`*.freezed.dart` são excluídos, e `invalid_annotation_target` é rebaixado para permitir freezed + json_serializable. Espera-se que código novo passe limpo no `flutter analyze`.
- **Testes**: `flutter_test` para widget tests; `mocktail` está disponível para mocking.

## Notas de plataforma

- namespace/applicationId Android: `br.com.malcong.tccelta_mobile`; Gradle é Kotlin DSL (`android/app/build.gradle.kts`), Java/Kotlin 17. O release atualmente assina com a chave de debug (placeholder — precisa de uma config de assinatura real antes de publicar).
- Apenas os targets Android e iOS estão configurados.
