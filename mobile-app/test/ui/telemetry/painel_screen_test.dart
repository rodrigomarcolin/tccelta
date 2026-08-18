import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_adapter_info.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_reading.dart';
import 'package:tccelta_mobile/src/domain/repositories/obd2_repository.dart';
import 'package:tccelta_mobile/src/ui/connection/connection_providers.dart';
import 'package:tccelta_mobile/src/ui/telemetry/telemetry_providers.dart';
import 'package:tccelta_mobile/src/ui/telemetry/view/painel_screen.dart';

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
  Widget wrap({List<Obd2Reading> readings = const []}) => ProviderScope(
        overrides: [
          bleServiceProvider.overrideWithValue(FakeBleService()),
          obd2RepositoryProvider
              .overrideWithValue(_FakeObd2Repository(readings: readings)),
        ],
        child: MaterialApp(theme: AppTheme.dark, home: const PainelScreen()),
      );

  Finder toggleFinder(Obd2Pid pid) =>
      find.byKey(ValueKey('sensor_toggle_${pid.name}'));

  /// Abre o sheet de sensores, filtra por [query] e toca no alternador
  /// (`+`/check) de [pid].
  Future<void> openAndToggle(
    WidgetTester tester,
    String query,
    Obd2Pid pid,
  ) async {
    await tester.tap(find.text('Adicionar indicador').first);
    await tester.pumpAndSettle();

    await tester.enterText(find.byType(TextField), query);
    await tester.pumpAndSettle();

    await tester.tap(toggleFinder(pid));
    await tester.pumpAndSettle();
  }

  testWidgets('painel vazio mostra o estado vazio e o CTA', (tester) async {
    await tester.pumpWidget(wrap());
    await tester.pump(const Duration(milliseconds: 10));

    expect(find.text('Painel vazio'), findsOneWidget);
    expect(find.text('Adicionar indicador'), findsOneWidget);
  });

  testWidgets('botão OK fecha o sheet de sensores', (tester) async {
    await tester.pumpWidget(wrap());
    await tester.pump(const Duration(milliseconds: 10));

    await tester.tap(find.text('Adicionar indicador').first);
    await tester.pumpAndSettle();
    expect(find.byType(TextField), findsOneWidget); // campo de busca do sheet

    await tester.tap(find.text('OK'));
    await tester.pumpAndSettle();

    expect(find.byType(TextField), findsNothing);
  });

  testWidgets('adicionar um indicador faz ele aparecer no painel',
      (tester) async {
    await tester.pumpWidget(
      wrap(readings: const [Obd2Reading(pid: Obd2Pid.rpm, value: 1500)]),
    );
    await tester.pump(const Duration(milliseconds: 10));

    // Filtra pelo PID (dígitos hex) para garantir um único resultado.
    await openAndToggle(tester, '010C', Obd2Pid.rpm);

    // Fecha o sheet tocando na barreira.
    await tester.tapAt(const Offset(10, 10));
    await tester.pumpAndSettle();

    expect(find.text('ROTAÇÃO DO MOTOR'), findsOneWidget);
    expect(find.text('1500'), findsOneWidget);
    expect(find.text('Painel vazio'), findsNothing);
  });

  testWidgets('remover um indicador (com confirmação) tira ele do painel',
      (tester) async {
    await tester.pumpWidget(
      wrap(readings: const [Obd2Reading(pid: Obd2Pid.rpm, value: 1500)]),
    );
    await tester.pump(const Duration(milliseconds: 10));

    await openAndToggle(tester, '010C', Obd2Pid.rpm);
    await tester.tapAt(const Offset(10, 10));
    await tester.pumpAndSettle();
    expect(find.text('ROTAÇÃO DO MOTOR'), findsOneWidget);

    // Reabre o sheet, filtra pelo mesmo sensor (agora já adicionado) e toca
    // no check para pedir a remoção.
    await openAndToggle(tester, '010C', Obd2Pid.rpm);
    expect(find.text('Remover Rotação do motor?'), findsOneWidget);

    await tester.tap(find.text('Remover'));
    await tester.pumpAndSettle();

    // Fecha o sheet.
    await tester.tapAt(const Offset(10, 10));
    await tester.pumpAndSettle();

    expect(find.text('ROTAÇÃO DO MOTOR'), findsNothing);
    expect(find.text('Painel vazio'), findsOneWidget);
  });

  testWidgets('busca filtra pelo nome e pelo código do PID', (tester) async {
    await tester.pumpWidget(wrap());
    await tester.pump(const Duration(milliseconds: 10));

    await tester.tap(find.text('Adicionar indicador').first);
    await tester.pumpAndSettle();

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

  testWidgets('arrastar um indicador troca sua posição com o outro',
      (tester) async {
    await tester.pumpWidget(
      wrap(
        readings: const [
          Obd2Reading(pid: Obd2Pid.rpm, value: 1500),
          Obd2Reading(pid: Obd2Pid.speed, value: 60),
        ],
      ),
    );
    await tester.pump(const Duration(milliseconds: 10));

    await openAndToggle(tester, '010C', Obd2Pid.rpm);
    await tester.enterText(find.byType(TextField), '010D');
    await tester.pumpAndSettle();
    await tester.tap(toggleFinder(Obd2Pid.speed));
    await tester.pumpAndSettle();
    await tester.tapAt(const Offset(10, 10)); // fecha o sheet
    await tester.pumpAndSettle();

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
}
