import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/domain/repositories/permissions_repository.dart';
import 'package:tccelta_mobile/src/router/app_router.dart';
import 'package:tccelta_mobile/src/router/app_routes.dart';
import 'package:tccelta_mobile/src/ui/connection/connection_providers.dart';
import 'package:tccelta_mobile/src/ui/connection/widgets/connection_guard.dart';

import '../../../support/fake_ble_service.dart';

/// [PermissionsRepository] falso com resposta fixa de permissão.
class _FakePermissions implements PermissionsRepository {
  _FakePermissions({required this.granted});

  final bool granted;

  @override
  Future<bool> hasBluetoothPermission() async => granted;

  @override
  Future<bool> requestBluetoothPermission() async => granted;
}

void main() {
  // Replica o wiring de produção (main.dart): o guard envolve o subtree de
  // rotas via o `builder` do MaterialApp.router. O adaptador fica `on` para o
  // caminho de permissão não ser mascarado pelo gate de "BT desligado".
  Widget app({required bool granted}) => ProviderScope(
    overrides: [
      bleServiceProvider.overrideWithValue(FakeBleService()),
      permissionsRepositoryProvider.overrideWithValue(
        _FakePermissions(granted: granted),
      ),
    ],
    child: MaterialApp.router(
      theme: AppTheme.dark,
      routerConfig: appRouter,
      builder: (context, child) => ConnectionGuard(child: child!),
    ),
  );

  // Simula ir aos Ajustes do SO e voltar: resumed -> inactive -> resumed
  // dispara o `onResume` do AppLifecycleListener.
  Future<void> resumeApp(WidgetTester tester) async {
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.inactive);
    await tester.pump();
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
    // Dá turnos ao callback assíncrono (checar permissão + disconnect + go).
    await tester.pump();
    await tester.pump();
    await tester.pump();
  }

  testWidgets(
    'permissão revogada no resume, estando no painel -> permissions',
    (tester) async {
      appRouter.go(AppRoutes.painel);
      await tester.pumpWidget(app(granted: false));
      await tester.pump();
      expect(appRouter.state.matchedLocation, AppRoutes.painel);

      await resumeApp(tester);

      expect(appRouter.state.matchedLocation, AppRoutes.permissions);
    },
  );

  testWidgets('permissão mantida no resume, no painel -> fica no painel', (
    tester,
  ) async {
    appRouter.go(AppRoutes.painel);
    await tester.pumpWidget(app(granted: true));
    await tester.pump();
    expect(appRouter.state.matchedLocation, AppRoutes.painel);

    await resumeApp(tester);

    expect(appRouter.state.matchedLocation, AppRoutes.painel);
  });
}
