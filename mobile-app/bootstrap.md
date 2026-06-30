# Plano de Bootstrap — Projeto Flutter (arquitetura em camadas / MVVM)

> Brief para um agente de código (Claude Code) levantar o projeto do zero.
> Execute as fases em ordem. Ao fim de cada fase, rode `flutter analyze` e garanta que está limpo antes de seguir.
> **Regra de ouro:** dependências só apontam "pra dentro" → `ui` depende de `domain`; `data`/`services` dependem de `domain`; `domain` não depende de ninguém. A `view` NUNCA contém regra de negócio nem acessa `data` diretamente — só fala com o `view_model`.

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
    domain/                  # MODELOS DE DOMÍNIO compartilhados (entities) — sem deps externas
      models/
    data/                    # CAMADA DATA — obtenção de dados (reutilizável entre módulos)
      datasources/           # wrap de endpoints/plugins  (o guia oficial chama isto de "services")
      dtos/                  # modelos de transporte (freezed + json_serializable)
      repositories/          # SOURCE OF TRUTH; mapeiam DTO -> domain model; cache/erros/retry
    services/                # CAMADA SERVICES — side-effects / integrações externas
      analytics_service.dart #   (analytics, notificações, storage, etc.)
      notification_service.dart
    ui/                      # CAMADA UI — organizada por MÓDULO (feature-first)
      <modulo>/
        view/                # <modulo>_screen.dart  -> View "burra" (só observa + renderiza)
        view_model/          # <modulo>_view_model.dart -> ViewModel (lógica isolada = ex-"hook")
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

## 6. Testes (definição mínima)

- Teste unitário do `ProductsRepository` com `dio` mockado (`mocktail`), cobrindo sucesso e mapeamento de erro → `Failure`.
- Teste do `ProductsViewModel` com repository mockado, validando transição loading → data e o caso de erro (usar `ProviderContainer` para overrides).
- 1 widget test da `ProductsScreen` renderizando os 3 estados.

## 7. Definition of Done

- [ ] `flutter analyze` sem warnings/erros.
- [ ] `dart run build_runner build --delete-conflicting-outputs` gera sem conflito.
- [ ] `flutter test` verde.
- [ ] App roda em iOS e Android mostrando a lista de produtos com loading/erro/refresh.
- [ ] `view/` sem nenhuma chamada a `data/` ou regra de negócio (revisar imports).
- [ ] `domain/` sem imports de `dio`, `flutter`, ou pacotes de infra.
- [ ] README com o diagrama de camadas + a tabela de mapeamento RN→Flutter + comando de code-gen.

## Convenções (enforce em review)
- Sufixos de arquivo: `_screen.dart`, `_view_model.dart`, `_repository.dart`, `_datasource.dart`, `_dto.dart`.
- Um módulo de UI = exatamente uma `view` + um `view_model`.
- `data/` e `services/` são reutilizáveis entre módulos (organizados por tipo); `ui/` é organizada por módulo.
- Nada de `BuildContext` dentro de `view_model`.
- Segredos só via `--dart-define` / `envied`; nunca hardcoded nem commitados.

## Fora de escopo deste bootstrap
CI/CD, code signing, push notifications reais, integração com backend específico, i18n — tratar em planos separados depois que o esqueleto estiver verde.