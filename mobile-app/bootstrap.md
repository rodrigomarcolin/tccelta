# Plano de Bootstrap — Projeto Flutter (arquitetura em camadas / MVVM)

> Brief para um agente de código (Claude Code) levantar o projeto do zero.
> Execute as fases em ordem. Ao fim de cada fase, rode `flutter analyze` e garanta que está limpo antes de seguir.
> **Regra de ouro (direção das dependências):** tudo aponta "pra dentro", terminando nos models puros:
> `ui → application → data → domain`.
> - `domain/` (só models) NÃO depende de ninguém — nem de `flutter`, `dio`, DTOs ou qualquer infra. É importável por todas as camadas justamente por ser puro.
> - `application/` (use cases) pode depender de `data` (repositories concretos) e de `domain`.
> - `data/` depende de `domain`; `services/` (side-effects) é transversal.
> - A `view` NUNCA contém regra de negócio nem acessa `data`/`application` de fora do seu `view_model` — só fala com o `view_model`.
> - O `view_model` chama um **use case** (`application/`) quando há regra de negócio/orquestração, ou o **repository** (`data/`) direto quando é acesso simples a dados.

---

## 0. Pré-requisitos (verificar, não assumir)

```bash
flutter --version        # exigir Flutter 3.44+ / Dart 3.x
flutter doctor           # resolver qualquer ❌ antes de continuar
```
- Recomendado fixar a versão com FVM (`fvm install` + `.fvmrc`) se o time quiser reprodutibilidade. Opcional.

## 1. Criar o projeto

```bash
flutter create --org com.suaempresa --project-name app_nome app_nome
cd app_nome
```
- Remover o conteúdo de exemplo de `lib/main.dart` e o `test/widget_test.dart` default (serão reescritos).

## 2. Dependências (locked decisions)

Rodar exatamente:

```bash
# Estado / ViewModel (default: Riverpod 3.x). Trocar por flutter_bloc se o time decidir.
flutter pub add flutter_riverpod
flutter pub add hooks_riverpod flutter_hooks   # opcional: "sensação de hook" (useState/useEffect)

# Navegação
flutter pub add go_router

# Rede
flutter pub add dio

# Modelos imutáveis + serialização
flutter pub add freezed_annotation json_annotation
flutter pub add dev:build_runner dev:freezed dev:json_serializable

# Storage
flutter pub add shared_preferences

# Qualidade / testes
flutter pub add dev:very_good_analysis dev:mocktail
```

> **Alternativa BLoC:** se optar por BLoC, troque o bloco de estado por
> `flutter pub add flutter_bloc bloc` (+ `dev:bloc_test`) e adicione `flutter pub add get_it injectable dev:injectable_generator` para DI.
> A estrutura de pastas abaixo NÃO muda — só a implementação dentro de `view_model/`.

## 3. Configuração base

**`analysis_options.yaml`** (na raiz):
```yaml
include: package:very_good_analysis/analysis_options.yaml
analyzer:
  errors:
    invalid_annotation_target: ignore   # necessário p/ freezed + json_serializable
  exclude:
    - "**/*.g.dart"
    - "**/*.freezed.dart"
```

**Comando de code-gen** (deixar rodando durante o dev):
```bash
dart run build_runner watch --delete-conflicting-outputs
```

**Flavors / ambientes:** usar `--dart-define` (não commitar segredos). Criar `main_development.dart` e `main_production.dart` que apenas definem o ambiente e chamam `bootstrap()`.

## 4. Estrutura de pastas (criar com placeholders)

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

### Mapeamento RN → Flutter (manter como referência viva no README)
| React Native | Flutter |
|---|---|
| camada `data` (obtenção de dados) | `src/data/` (datasources + dtos + repositories) |
| camada `services` (side-effects/integrações) | `src/services/` |
| regra de negócio reutilizável / que cruza fontes | `src/application/` (use cases) |
| tipos/models compartilhados | `src/domain/models/` (puro) |
| custom hooks (lógica fora da tela) | `view_model/` (Riverpod `AsyncNotifier`) |
| componentes genéricos (página única) | `src/ui/core/widgets/` |
| componentes específicos do módulo | `src/ui/<modulo>/widgets/` |
| tela sem regra de negócio | `view/<modulo>_screen.dart` |

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

## 7. Definition of Done

- [ ] `flutter analyze` sem warnings/erros.
- [ ] `dart run build_runner build --delete-conflicting-outputs` gera sem conflito.
- [ ] `flutter test` verde.
- [ ] App roda em iOS e Android mostrando a lista de produtos com loading/erro/refresh.
- [ ] `view/` sem nenhuma chamada a `data/`/`application/` ou regra de negócio (revisar imports).
- [ ] `domain/` sem imports de `dio`, `flutter`, DTOs ou qualquer pacote de infra (camada pura).
- [ ] `application/` (se existir) depende só de `data` (repositories) e `domain` — nunca de datasource nem de `flutter`/`ui`.
- [ ] Nenhum use case "passthrough" (que só repassa pro repository).
- [ ] README com o diagrama de camadas + a tabela de mapeamento RN→Flutter + comando de code-gen.

## Convenções (enforce em review)
- Sufixos de arquivo: `_screen.dart`, `_view_model.dart`, `_use_case.dart`, `_repository.dart`, `_datasource.dart`, `_dto.dart`.
- Um módulo de UI = exatamente uma `view` + um `view_model`.
- `domain/` é puro (só models, zero deps); `data/` e `services/` são reutilizáveis entre módulos (por tipo); `application/` e `ui/` são organizados por módulo.
- `view_model` chama use case (`application/`) OU repository (`data/`) — nunca datasource.
- Use case só nasce quando cruza repositories ou é reusado; caso contrário, `view_model → repository` direto.
- Nada de `BuildContext` dentro de `view_model` nem de use case.
- Segredos só via `--dart-define` / `envied`; nunca hardcoded nem commitados.

## Fora de escopo deste bootstrap
CI/CD, code signing, push notifications reais, integração com backend específico, i18n — tratar em planos separados depois que o esqueleto estiver verde.