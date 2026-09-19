import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/router/app_router.dart';
import 'package:tccelta_mobile/src/router/app_routes.dart';
import 'package:tccelta_mobile/src/ui/connection/connection_providers.dart';
import 'package:tccelta_mobile/src/ui/telemetry/telemetry_providers.dart';

import '../../support/fake_ble_service.dart';
import '../../support/fake_obd2_repository.dart';

void main() {
  // Usa o `appRouter` real (`MaterialApp.router`): a tab bar da tela "Mais"
  // navega para as outras abas via `context.go`, que depende de um
  // `GoRouter` no contexto (mesmo padrão de `painel_screen_test.dart`).
  Widget app() => ProviderScope(
    overrides: [
      bleServiceProvider.overrideWithValue(FakeBleService()),
      obd2RepositoryProvider.overrideWithValue(FakeObd2Repository()),
    ],
    child: MaterialApp.router(theme: AppTheme.dark, routerConfig: appRouter),
  );

  setUp(() => appRouter.go(AppRoutes.more));

  testWidgets('tocar "Sensores" na tab bar abre a lista de sensores', (
    tester,
  ) async {
    await tester.pumpWidget(app());
    await tester.pump(const Duration(milliseconds: 10));

    await tester.tap(find.text('Sensores'));
    await tester.pumpAndSettle();

    expect(find.byType(TextField), findsOneWidget);
  });

  testWidgets('tocar "Painel" na tab bar volta ao painel', (tester) async {
    await tester.pumpWidget(app());
    await tester.pump(const Duration(milliseconds: 10));

    await tester.tap(find.text('Painel'));
    await tester.pumpAndSettle();

    expect(find.text('Painel vazio'), findsOneWidget);
  });

  testWidgets('tocar "DTCs" na tab bar abre a aba de diagnóstico', (
    tester,
  ) async {
    await tester.pumpWidget(app());
    await tester.pump(const Duration(milliseconds: 10));

    await tester.tap(find.text('DTCs'));
    await tester.pumpAndSettle();

    expect(find.text('Diagnóstico'), findsOneWidget);
  });
}
