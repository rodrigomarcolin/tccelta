import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_active_entry.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_snapshot.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_status.dart';
import 'package:tccelta_mobile/src/router/app_router.dart';
import 'package:tccelta_mobile/src/router/app_routes.dart';
import 'package:tccelta_mobile/src/ui/connection/connection_providers.dart';
import 'package:tccelta_mobile/src/ui/telemetry/telemetry_providers.dart';

import '../../support/fake_ble_service.dart';
import '../../support/fake_obd2_repository.dart';

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
    expect(find.text('Acesa'), findsOneWidget); // luz de injeção
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
}
