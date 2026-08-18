import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_adapter_info.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_reading.dart';
import 'package:tccelta_mobile/src/domain/repositories/obd2_repository.dart';
import 'package:tccelta_mobile/src/router/app_router.dart';
import 'package:tccelta_mobile/src/router/app_routes.dart';
import 'package:tccelta_mobile/src/ui/connection/connection_providers.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';
import 'package:tccelta_mobile/src/ui/telemetry/telemetry_providers.dart';

import '../../support/fake_ble_service.dart';

/// Repository de telemetria fake que devolve leituras fixas (sem I/O).
class _FakeObd2Repository implements Obd2Repository {
  _FakeObd2Repository({this.readings = const []});

  final List<Obd2Reading> readings;

  @override
  List<Obd2Pid> get pids => Obd2Pid.values;

  @override
  Obd2AdapterInfo? get adapterInfo => null;

  @override
  Future<void> initialize() async {}

  @override
  Future<Set<Obd2Pid>> discoverSupported() async => Obd2Pid.values.toSet();

  @override
  Future<Obd2Reading> read(Obd2Pid pid) async =>
      readings.firstWhere((r) => r.pid == pid);

  @override
  Future<List<Obd2Reading>> readAll() async => readings;
}

void main() {
  // Usa o `appRouter` real (`MaterialApp.router`), não `MaterialApp(home: ...)`
  // direto: a tela de sensores é uma rota empilhada de verdade
  // (`context.push`), que depende de um `GoRouter` no contexto.
  Widget app({List<Obd2Reading> readings = const []}) => ProviderScope(
    overrides: [
      bleServiceProvider.overrideWithValue(FakeBleService()),
      obd2RepositoryProvider.overrideWithValue(
        _FakeObd2Repository(readings: readings),
      ),
    ],
    child: MaterialApp.router(theme: AppTheme.dark, routerConfig: appRouter),
  );

  Finder toggleFinder(Obd2Pid pid) =>
      find.byKey(ValueKey('sensor_toggle_${pid.name}'));

  /// Abre a tela de sensores (empilhada) e filtra por [query].
  Future<void> openSheet(WidgetTester tester, [String query = '']) async {
    await tester.tap(find.text('Adicionar indicador').first);
    await tester.pumpAndSettle();
    if (query.isNotEmpty) {
      await tester.enterText(find.byType(TextField), query);
      await tester.pumpAndSettle();
    }
  }

  /// Toca na linha do sensor [pid] (não no ícone) — abre o sheet de formato,
  /// direto (fluxo de adicionar ou de editar, conforme já esteja no painel).
  Future<void> openFormatFor(WidgetTester tester, Obd2Pid pid) async {
    await tester.tap(find.text(pid.label));
    await tester.pumpAndSettle();
  }

  /// Toca no ícone quadrado de alternância de [pid] (não na linha).
  Future<void> tapToggleIcon(WidgetTester tester, Obd2Pid pid) async {
    await tester.tap(toggleFinder(pid));
    await tester.pumpAndSettle();
  }

  Future<void> tapPrimary(WidgetTester tester) async {
    await tester.tap(find.byKey(const Key('indicator_format_primary')));
    await tester.pumpAndSettle();
  }

  /// Fecha a tela de sensores (rota empilhada) pela seta de voltar da AppBar.
  Future<void> closeSensorScreen(WidgetTester tester) async {
    await tester.tap(find.byIcon(Icons.arrow_back_ios_new));
    await tester.pumpAndSettle();
  }

  /// Adiciona [pid] com o formato padrão (Número): abre a tela de sensores,
  /// abre o sheet de formato pela linha, confirma sem mexer em nada (já é o
  /// último passo — o sheet fecha sozinho), e volta ao Painel.
  Future<void> addWithDefaultFormat(
    WidgetTester tester,
    String query,
    Obd2Pid pid,
  ) async {
    await openSheet(tester, query);
    await openFormatFor(tester, pid);
    await tapPrimary(tester); // 'Adicionar' — Número não tem passo de escala.
    await closeSensorScreen(tester);
  }

  setUp(() => appRouter.go(AppRoutes.painel));

  testWidgets('painel vazio mostra o estado vazio e o CTA', (tester) async {
    await tester.pumpWidget(app());
    await tester.pump(const Duration(milliseconds: 10));

    expect(find.text('Painel vazio'), findsOneWidget);
    expect(find.text('Adicionar indicador'), findsOneWidget);
  });

  testWidgets('seta de voltar fecha a tela de sensores', (tester) async {
    await tester.pumpWidget(app());
    await tester.pump(const Duration(milliseconds: 10));

    await tester.tap(find.text('Adicionar indicador').first);
    await tester.pumpAndSettle();
    // Campo de busca da tela de sensores.
    expect(find.byType(TextField), findsOneWidget);

    await closeSensorScreen(tester);

    expect(find.byType(TextField), findsNothing);
  });

  testWidgets('botão OK fecha a tela de sensores', (tester) async {
    await tester.pumpWidget(app());
    await tester.pump(const Duration(milliseconds: 10));

    await tester.tap(find.text('Adicionar indicador').first);
    await tester.pumpAndSettle();
    expect(find.byType(TextField), findsOneWidget);

    await tester.tap(find.text('OK'));
    await tester.pumpAndSettle();

    expect(find.byType(TextField), findsNothing);
  });

  testWidgets(
    'tocar na linha do sensor abre o sheet de escolha de formato, direto',
    (tester) async {
      await tester.pumpWidget(app());
      await tester.pump(const Duration(milliseconds: 10));

      await openSheet(tester, '010C');
      await openFormatFor(tester, Obd2Pid.rpm);

      // Tela de formato, sem nenhuma tela de detalhe/gráfico no meio.
      expect(find.text('Exibir no painel'), findsOneWidget);
      expect(find.text('Gauge'), findsOneWidget);
      expect(find.text('Histórico — gráfico'), findsOneWidget);
    },
  );

  testWidgets('adicionar um indicador com o formato padrão faz ele aparecer no '
      'painel', (tester) async {
    await tester.pumpWidget(
      app(readings: const [Obd2Reading(pid: Obd2Pid.rpm, value: 1500)]),
    );
    await tester.pump(const Duration(milliseconds: 10));

    await addWithDefaultFormat(tester, '010C', Obd2Pid.rpm);

    expect(find.text('ROTAÇÃO DO MOTOR'), findsOneWidget);
    expect(find.text('1500'), findsOneWidget);
    expect(find.text('Painel vazio'), findsNothing);
  });

  testWidgets(
    'tocar num card já no painel reabre a tela de formato pré-preenchida',
    (tester) async {
      await tester.pumpWidget(
        app(readings: const [Obd2Reading(pid: Obd2Pid.rpm, value: 1500)]),
      );
      await tester.pump(const Duration(milliseconds: 10));
      await addWithDefaultFormat(tester, '010C', Obd2Pid.rpm);

      await tester.tap(find.text('ROTAÇÃO DO MOTOR'));
      await tester.pumpAndSettle();

      expect(find.text('Exibir no painel'), findsOneWidget);
      expect(find.text('Salvar'), findsOneWidget); // edição, não adição
      expect(find.text('Remover do painel'), findsOneWidget);
    },
  );

  testWidgets(
    'remover um indicador (tocando no ícone, com confirmação) tira ele do '
    'painel',
    (tester) async {
      await tester.pumpWidget(
        app(readings: const [Obd2Reading(pid: Obd2Pid.rpm, value: 1500)]),
      );
      await tester.pump(const Duration(milliseconds: 10));
      await addWithDefaultFormat(tester, '010C', Obd2Pid.rpm);
      expect(find.text('ROTAÇÃO DO MOTOR'), findsOneWidget);

      // Reabre o sheet, filtra pelo mesmo sensor (já adicionado) e toca
      // especificamente no ícone — não na linha — para remover.
      await openSheet(tester, '010C');
      await tapToggleIcon(tester, Obd2Pid.rpm);
      expect(find.text('Remover Rotação do motor?'), findsOneWidget);

      await tester.tap(find.text('Remover'));
      await tester.pumpAndSettle();
      await closeSensorScreen(tester);

      expect(find.text('ROTAÇÃO DO MOTOR'), findsNothing);
      expect(find.text('Painel vazio'), findsOneWidget);
    },
  );

  testWidgets('busca filtra pelo nome e pelo código do PID', (tester) async {
    await tester.pumpWidget(app());
    await tester.pump(const Duration(milliseconds: 10));

    await openSheet(tester);

    // Todos os sensores aparecem sem filtro.
    expect(find.text('Velocidade'), findsOneWidget);
    expect(find.text('Rotação do motor'), findsOneWidget);

    // Filtro por fragmento do NOME.
    await tester.enterText(find.byType(TextField), 'Rotação');
    await tester.pumpAndSettle();
    expect(find.text('Rotação do motor'), findsOneWidget);
    expect(find.text('Velocidade'), findsNothing);

    // Filtro por fragmento do CÓDIGO (hex), case-insensitive.
    await tester.enterText(find.byType(TextField), '010d');
    await tester.pumpAndSettle();
    expect(find.text('Velocidade'), findsOneWidget);
    expect(find.text('Rotação do motor'), findsNothing);
  });

  testWidgets('arrastar um indicador troca sua posição com o outro', (
    tester,
  ) async {
    await tester.pumpWidget(
      app(
        readings: const [
          Obd2Reading(pid: Obd2Pid.rpm, value: 1500),
          Obd2Reading(pid: Obd2Pid.speed, value: 60),
        ],
      ),
    );
    await tester.pump(const Duration(milliseconds: 10));

    await addWithDefaultFormat(tester, '010C', Obd2Pid.rpm);
    await addWithDefaultFormat(tester, '010D', Obd2Pid.speed);

    final rpmCenterBefore = tester.getCenter(find.text('ROTAÇÃO DO MOTOR'));
    final speedCenterBefore = tester.getCenter(find.text('VELOCIDADE'));
    expect(rpmCenterBefore.dx, lessThan(speedCenterBefore.dx));

    final gesture = await tester.startGesture(rpmCenterBefore);
    // Dispara o long-press que ativa o `LongPressDraggable`.
    await tester.pump(const Duration(milliseconds: 600));
    await gesture.moveTo(speedCenterBefore);
    await tester.pump();
    await gesture.up();
    await tester.pumpAndSettle();

    final rpmCenterAfter = tester.getCenter(find.text('ROTAÇÃO DO MOTOR'));
    final speedCenterAfter = tester.getCenter(find.text('VELOCIDADE'));
    expect(speedCenterAfter.dx, lessThan(rpmCenterAfter.dx));
  });

  group('tamanho no grid por formato', () {
    testWidgets(
      'histórico e número-linha-inteira ocupam a linha inteira do grid',
      (tester) async {
        await tester.pumpWidget(
          app(
            readings: const [
              Obd2Reading(pid: Obd2Pid.rpm, value: 1500),
              Obd2Reading(pid: Obd2Pid.speed, value: 60),
            ],
          ),
        );
        await tester.pump(const Duration(milliseconds: 10));

        // rpm no formato padrão (Número, meia coluna).
        await addWithDefaultFormat(tester, '010C', Obd2Pid.rpm);

        // speed no formato Histórico (linha inteira).
        await openSheet(tester, '010D');
        await openFormatFor(tester, Obd2Pid.speed);
        await tester.tap(find.text('Histórico — gráfico'));
        await tester.pumpAndSettle();
        await tapPrimary(tester); // 'Continuar' -> passo de escala.
        await tapPrimary(tester); // 'Adicionar'.
        await closeSensorScreen(tester);

        final rpmWidth = tester.getSize(find.text('ROTAÇÃO DO MOTOR')).width;
        final rpmCardWidth = tester
            .getSize(
              find.ancestor(
                of: find.text('ROTAÇÃO DO MOTOR'),
                matching: find.byType(StatCard),
              ),
            )
            .width;
        // StatGraphCard mostra o rótulo tal qual (sem uppercase), diferente do
        // StatCard.value.
        final historyCardWidth = tester
            .getSize(
              find.ancestor(
                of: find.text('Velocidade'),
                matching: find.byType(StatGraphCard),
              ),
            )
            .width;

        // O card de histórico ocupa bem mais que o meia-coluna do número.
        expect(historyCardWidth, greaterThan(rpmCardWidth * 1.5));
        // Sanity: o rótulo do número cabe dentro do seu card (meia coluna).
        expect(rpmWidth, lessThan(rpmCardWidth));
      },
    );
  });

  testWidgets('alça de arrastar fica dentro do card (mesmo o mais curto) em '
      'viewport de telefone real', (tester) async {
    tester.view.physicalSize = const Size(392, 806);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    await tester.pumpWidget(
      app(
        readings: const [
          Obd2Reading(pid: Obd2Pid.rpm, value: 1500),
          Obd2Reading(pid: Obd2Pid.speed, value: 60),
        ],
      ),
    );
    await tester.pump(const Duration(milliseconds: 10));

    await addWithDefaultFormat(tester, '010C', Obd2Pid.rpm);
    await addWithDefaultFormat(tester, '010D', Obd2Pid.speed);

    expect(tester.takeException(), isNull);

    // O card de "Velocidade" (rótulo + valor mais curtos) é o caso mais
    // provável de sobrar altura de sobra — ainda assim a alça precisa caber
    // dentro do card, e o card precisa ter ao menos uma posição de grid.
    // A alça é sobreposta ao card (não um filho dele) no mesmo `Stack` da
    // célula do grid — o `Stack` (sem `Positioned`) assume o tamanho do card,
    // então seu retângulo já serve como o retângulo do card.
    final speedCell = find
        .ancestor(of: find.text('VELOCIDADE'), matching: find.byType(Stack))
        .first;
    final cardRect = tester.getRect(speedCell);
    final handleRect = tester.getRect(
      find.descendant(
        of: speedCell,
        matching: find.byType(DragHandleDots),
      ),
    );

    expect(cardRect.height, greaterThanOrEqualTo(86));
    expect(cardRect.contains(handleRect.topLeft), isTrue);
    expect(cardRect.contains(handleRect.bottomRight), isTrue);
  });
}
