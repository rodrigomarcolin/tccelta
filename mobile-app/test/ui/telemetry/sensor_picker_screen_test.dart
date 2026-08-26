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

  @override
  Future<List<Obd2Reading>> readMany(List<Obd2Pid> pids) async =>
      readings.where((r) => pids.contains(r.pid)).toList();
}

void main() {
  // Usa o `appRouter` real (`MaterialApp.router`), não `MaterialApp(home: ...)`
  // direto: a aba "Sensores" navega via `context.go` de verdade, e o modo
  // `fromTab: true` da `SensorPickerScreen` depende de um `GoRouter` no
  // contexto (mesmo padrão de `painel_screen_test.dart`).
  Widget app({List<Obd2Reading> readings = const []}) => ProviderScope(
    overrides: [
      bleServiceProvider.overrideWithValue(FakeBleService()),
      obd2RepositoryProvider.overrideWithValue(
        _FakeObd2Repository(readings: readings),
      ),
    ],
    child: MaterialApp.router(theme: AppTheme.dark, routerConfig: appRouter),
  );

  setUp(() => appRouter.go(AppRoutes.painel));

  /// Toca a aba "Sensores" da `AppTabBar`, no Painel.
  Future<void> goToSensoresTab(WidgetTester tester) async {
    await tester.tap(find.text('Sensores'));
    await tester.pumpAndSettle();
  }

  /// Filtra a lista pelo código do PID de [pid] e toca na linha — abre o
  /// picker de painéis (modo `fromTab: true`). O filtro traz o sensor pra
  /// dentro da viewport, sem precisar rolar a lista.
  Future<void> openSensorTile(WidgetTester tester, Obd2Pid pid) async {
    await tester.enterText(find.byType(TextField), pid.command);
    await tester.pumpAndSettle();
    await tester.tap(find.text(pid.label));
    await tester.pumpAndSettle();
  }

  /// Escolhe o painel [panelName] no picker de painéis.
  Future<void> pickPanel(WidgetTester tester, String panelName) async {
    await tester.tap(find.text(panelName));
    await tester.pumpAndSettle();
  }

  Future<void> tapPrimary(WidgetTester tester) async {
    await tester.tap(find.byKey(const Key('indicator_format_primary')));
    await tester.pumpAndSettle();
  }

  /// Cria um painel novo pela linha de abas do Painel (fica ativo).
  Future<void> createPanelFromPainel(WidgetTester tester) async {
    await tester.tap(find.byKey(const ValueKey('panel_tabs_create')));
    await tester.pumpAndSettle();
  }

  testWidgets('aba "Sensores" abre a lista de sensores, sem botão OK', (
    tester,
  ) async {
    await tester.pumpWidget(app());
    await tester.pump(const Duration(milliseconds: 10));

    await goToSensoresTab(tester);

    expect(find.byType(TextField), findsOneWidget); // campo de busca
    expect(find.text('OK'), findsNothing); // raiz de aba, sem botão OK
  });

  testWidgets(
    'tocar um sensor abre o picker de painéis, listando os painéis',
    (tester) async {
      await tester.pumpWidget(app());
      await tester.pump(const Duration(milliseconds: 10));

      await goToSensoresTab(tester);
      await openSensorTile(tester, Obd2Pid.rpm);

      expect(find.text('Adicionar a qual painel?'), findsOneWidget);
      expect(find.text('Painel 1'), findsOneWidget);
      expect(find.text('JÁ ADICIONADO'), findsNothing);
    },
  );

  testWidgets(
    'escolher um painel sem o indicador abre o sheet em modo adicionar e '
    'adiciona só nele — sem afetar o painel ativo',
    (tester) async {
      await tester.pumpWidget(
        app(readings: const [Obd2Reading(pid: Obd2Pid.rpm, value: 1500)]),
      );
      await tester.pump(const Duration(milliseconds: 10));

      await createPanelFromPainel(tester); // "Painel 2" fica ativo
      await goToSensoresTab(tester);
      await openSensorTile(tester, Obd2Pid.rpm);
      await pickPanel(tester, 'Painel 1'); // não é o painel ativo

      expect(find.text('Exibir no painel'), findsOneWidget);
      expect(find.text('Adicionar'), findsOneWidget); // fluxo de adicionar

      await tapPrimary(tester); // 'Adicionar' — Número não tem passo de escala.

      // Volta pra lista de sensores — o sensor aparece marcado como
      // adicionado (em algum painel).
      expect(find.byKey(const ValueKey('sensor_toggle_rpm')), findsOneWidget);

      // O painel ativo (Painel 2) continua vazio — só o Painel 1 recebeu.
      await tester.tap(find.text('Painel'));
      await tester.pumpAndSettle();
      expect(find.text('Painel vazio'), findsOneWidget);

      await tester.tap(find.text('Painel 1'));
      await tester.pumpAndSettle();
      expect(find.text('ROTAÇÃO DO MOTOR'), findsOneWidget);
    },
  );

  testWidgets(
    'reabrir o picker mostra "JÁ ADICIONADO" no painel que recebeu o '
    'indicador, e escolhê-lo abre o sheet em modo editar',
    (tester) async {
      await tester.pumpWidget(
        app(readings: const [Obd2Reading(pid: Obd2Pid.rpm, value: 1500)]),
      );
      await tester.pump(const Duration(milliseconds: 10));

      await goToSensoresTab(tester);
      await openSensorTile(tester, Obd2Pid.rpm);
      await pickPanel(tester, 'Painel 1');
      await tapPrimary(tester); // adiciona com o formato padrão

      await openSensorTile(tester, Obd2Pid.rpm);
      expect(find.text('JÁ ADICIONADO'), findsOneWidget);

      await pickPanel(tester, 'Painel 1');

      expect(find.text('Exibir no painel'), findsOneWidget);
      expect(find.text('Salvar'), findsOneWidget); // edição, não adição
      expect(find.text('Remover do painel'), findsOneWidget);
    },
  );

  testWidgets('"Novo painel" no picker cria e usa um painel vazio na hora', (
    tester,
  ) async {
    await tester.pumpWidget(
      app(readings: const [Obd2Reading(pid: Obd2Pid.rpm, value: 1500)]),
    );
    await tester.pump(const Duration(milliseconds: 10));

    await goToSensoresTab(tester);
    await openSensorTile(tester, Obd2Pid.rpm);

    expect(find.text('Painel 1'), findsOneWidget);
    expect(find.text('Painel 2'), findsNothing);

    await tester.tap(find.text('Novo painel'));
    await tester.pumpAndSettle();

    // Foi direto pro sheet de formato, em modo adicionar — o painel novo
    // obviamente ainda não tem o indicador.
    expect(find.text('Exibir no painel'), findsOneWidget);
    expect(find.text('Adicionar'), findsOneWidget);

    await tapPrimary(tester);

    await tester.tap(find.text('Painel'));
    await tester.pumpAndSettle();

    // createPanel() já torna o painel novo o ativo.
    expect(find.text('Painel 2'), findsWidgets);
    expect(find.text('ROTAÇÃO DO MOTOR'), findsOneWidget);
  });
}
