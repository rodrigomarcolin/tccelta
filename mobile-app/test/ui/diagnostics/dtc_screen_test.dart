import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_code.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_component.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_severity.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_snapshot.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_status.dart';
import 'package:tccelta_mobile/src/domain/repositories/dtc_repository.dart';
import 'package:tccelta_mobile/src/router/app_router.dart';
import 'package:tccelta_mobile/src/router/app_routes.dart';
import 'package:tccelta_mobile/src/ui/connection/connection_providers.dart';
import 'package:tccelta_mobile/src/ui/diagnostics/diagnostics_providers.dart';

import '../../support/fake_ble_service.dart';

/// Repository de DTCs fake, com um catálogo fixo dado no construtor.
class _FakeDtcRepository implements DtcRepository {
  _FakeDtcRepository(this.codes, {this.milOn = false});

  final List<DtcCode> codes;
  final bool milOn;

  @override
  Future<DtcSnapshot> read() async => DtcSnapshot(codes: codes, milOn: milOn);
}

void main() {
  Widget app(List<DtcCode> codes, {bool milOn = false}) => ProviderScope(
    overrides: [
      bleServiceProvider.overrideWithValue(FakeBleService()),
      dtcRepositoryProvider.overrideWithValue(
        _FakeDtcRepository(codes, milOn: milOn),
      ),
    ],
    child: MaterialApp.router(theme: AppTheme.dark, routerConfig: appRouter),
  );

  setUp(() => appRouter.go(AppRoutes.dtc));

  const inactive = DtcCode(
    code: 'P0442',
    component: DtcComponent.emissions,
    name: 'Vazamento pequeno no sistema EVAP',
    severity: DtcSeverity.low,
  );

  const active = DtcCode(
    code: 'P0301',
    component: DtcComponent.engine,
    name: 'Falha de combustão — cilindro 1',
    severity: DtcSeverity.high,
    status: DtcStatus.confirmed,
  );

  testWidgets('sem códigos ativos mostra o estado vazio', (tester) async {
    await tester.pumpWidget(app([inactive]));
    await tester.pump(const Duration(milliseconds: 10));

    expect(find.text('Nenhum código ativo aqui'), findsOneWidget);
  });

  testWidgets('mostra o código e o nome dos DTCs ativos', (tester) async {
    await tester.pumpWidget(app([active, inactive], milOn: true));
    await tester.pump(const Duration(milliseconds: 10));

    expect(find.text('P0301'), findsOneWidget);
    expect(find.text('Falha de combustão — cilindro 1'), findsOneWidget);
    expect(find.text('CONFIRMADO'), findsOneWidget);
    expect(find.text('Acesa'), findsOneWidget); // luz de injeção
  });

  testWidgets('alternar para "Todos por componente" mostra o catálogo '
      'agrupado', (tester) async {
    await tester.pumpWidget(app([active, inactive]));
    await tester.pump(const Duration(milliseconds: 10));

    await tester.tap(find.text('Todos por componente'));
    await tester.pumpAndSettle();

    // "Motor"/"Emissões" aparecem tanto no chip de filtro quanto no
    // cabeçalho da seção — pelo menos um dos dois já confirma a vista.
    expect(find.text('Motor'), findsWidgets);
    expect(find.text('Emissões'), findsWidgets);
    // Ambos os códigos aparecem, ativo ou não, em suas seções de componente.
    expect(find.text('P0301'), findsOneWidget);
    expect(find.text('P0442'), findsOneWidget);
  });
}
