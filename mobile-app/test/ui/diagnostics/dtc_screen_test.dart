import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_active_entry.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_snapshot.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_status.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_adapter_info.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_reading.dart';
import 'package:tccelta_mobile/src/domain/repositories/obd2_repository.dart';
import 'package:tccelta_mobile/src/router/app_router.dart';
import 'package:tccelta_mobile/src/router/app_routes.dart';
import 'package:tccelta_mobile/src/ui/connection/connection_providers.dart';
import 'package:tccelta_mobile/src/ui/telemetry/telemetry_providers.dart';

import '../../support/fake_ble_service.dart';
import '../../support/fake_obd2_repository.dart';

/// [Obd2Repository] cujo [readDtc] só resolve quando [complete] é chamado —
/// permite observar o estado "carregando" do botão "Reler" no meio de uma
/// leitura, algo que o [FakeObd2Repository] (resolve na hora) não permite.
class _ControllableObd2Repository implements Obd2Repository {
  Completer<DtcSnapshot> _completer = Completer<DtcSnapshot>();

  /// Nº de leituras iniciadas.
  int readDtcCalls = 0;

  void complete(DtcSnapshot snapshot) => _completer.complete(snapshot);

  @override
  List<Obd2Pid> get pids => const [];

  @override
  Obd2AdapterInfo? get adapterInfo => null;

  @override
  Future<void> initialize() async {}

  @override
  Future<Set<Obd2Pid>> discoverSupported() async => const {};

  @override
  Future<Obd2Reading> read(Obd2Pid pid) => Future.error(UnimplementedError());

  @override
  Future<List<Obd2Reading>> readAll() async => const [];

  @override
  Future<List<Obd2Reading>> readMany(List<Obd2Pid> pids) async => const [];

  @override
  Future<DtcSnapshot> readDtc() {
    readDtcCalls++;
    _completer = Completer<DtcSnapshot>();
    return _completer.future;
  }
}

void main() {
  Widget app(List<DtcActiveEntry> active, {bool milOn = false}) =>
      ProviderScope(
        overrides: [
          bleServiceProvider.overrideWithValue(FakeBleService()),
          obd2RepositoryProvider.overrideWithValue(
            FakeObd2Repository(
              dtcSnapshot: DtcSnapshot(active: active, milOn: milOn),
            ),
          ),
        ],
        child: MaterialApp.router(
          theme: AppTheme.dark,
          routerConfig: appRouter,
        ),
      );

  setUp(() => appRouter.go(AppRoutes.dtc));

  // P0301 (motor) e P0442 (emissões) já existem no catálogo real
  // (`dtcCatalog`) — só o status ativo/inativo vem da leitura.
  const active = DtcActiveEntry(code: 'P0301', status: DtcStatus.confirmed);

  testWidgets('sem códigos ativos mostra o estado vazio', (tester) async {
    await tester.pumpWidget(app([]));
    await tester.pump(const Duration(milliseconds: 10));

    expect(find.text('Nenhum código ativo aqui'), findsOneWidget);
  });

  testWidgets('mostra o código e o nome dos DTCs ativos', (tester) async {
    await tester.pumpWidget(app([active], milOn: true));
    await tester.pump(const Duration(milliseconds: 10));

    expect(find.text('P0301'), findsOneWidget);
    expect(find.text('Falha de combustão — cilindro 1'), findsOneWidget);
    expect(find.text('CONFIRMADO'), findsOneWidget);
    expect(find.text('Acesa'), findsOneWidget); // luz de falha (MIL)
  });

  testWidgets('alternar para "Todos por componente" mostra o catálogo '
      'agrupado', (tester) async {
    await tester.pumpWidget(app([active]));
    await tester.pump(const Duration(milliseconds: 10));

    await tester.tap(find.text('Todos por componente'));
    await tester.pumpAndSettle();

    // "Motor"/"Emissões" aparecem tanto no chip de filtro quanto no
    // cabeçalho da seção — pelo menos um dos dois já confirma a vista.
    expect(find.text('Motor'), findsWidgets);
    expect(find.text('Emissões'), findsWidgets);
    // Ambos os códigos aparecem, ativo ou não, em suas seções de componente
    // — o catálogo inteiro (27 códigos) rola além da viewport de teste.
    expect(find.text('P0301'), findsOneWidget);
    await tester.scrollUntilVisible(
      find.text('P0442'),
      300,
      scrollable: find.byType(Scrollable).first,
    );
    expect(find.text('P0442'), findsOneWidget);
  });

  testWidgets('tocar "Reler" mostra carregando no botão até a leitura '
      'resolver', (tester) async {
    final repo = _ControllableObd2Repository();
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          bleServiceProvider.overrideWithValue(FakeBleService()),
          obd2RepositoryProvider.overrideWithValue(repo),
        ],
        child: MaterialApp.router(
          theme: AppTheme.dark,
          routerConfig: appRouter,
        ),
      ),
    );
    // Resolve a carga inicial disparada pelo build() do view model.
    repo.complete(const DtcSnapshot(active: [], milOn: false));
    await tester.pump(const Duration(milliseconds: 10));

    expect(find.text('Reler'), findsOneWidget);
    expect(find.byType(CircularProgressIndicator), findsNothing);

    // `pump(duration)`, não só `pump()`: dá tempo pro fade de "pressionado"
    // do `AppButton` completar (`_Pressable._minVisiblePress`), senão o
    // timer dele fica pendente no fim do teste.
    await tester.tap(find.text('Reler'));
    await tester.pump(const Duration(milliseconds: 100));

    // Em carregamento: texto/ícone somem, o spinner aparece, e o botão fica
    // desabilitado (2ª leitura não dispara antes da 1ª "Reler" resolver).
    expect(find.text('Reler'), findsNothing);
    expect(find.byType(CircularProgressIndicator), findsOneWidget);
    await tester.tap(find.byType(CircularProgressIndicator));
    await tester.pump(const Duration(milliseconds: 100));
    expect(repo.readDtcCalls, 2); // só a inicial + o toque em "Reler"

    repo.complete(const DtcSnapshot(active: [], milOn: false));
    await tester.pump(const Duration(milliseconds: 10));

    expect(find.text('Reler'), findsOneWidget);
    expect(find.byType(CircularProgressIndicator), findsNothing);
  });
}
