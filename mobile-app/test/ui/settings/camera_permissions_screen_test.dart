import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:go_router/go_router.dart';
import 'package:tccelta_mobile/src/domain/repositories/camera_permissions_repository.dart';
import 'package:tccelta_mobile/src/router/app_routes.dart';
import 'package:tccelta_mobile/src/ui/settings/settings_providers.dart';
import 'package:tccelta_mobile/src/ui/settings/view/camera_permissions_screen.dart';

/// Repository fake controlável (sem tocar no plugin real).
///
/// A rota de destino (`AppRoutes.pskScanQr`) é sempre um stub de texto aqui —
/// nunca a `QrScanScreen` real, que tocaria o canal de plataforma do
/// `mobile_scanner` (inexistente no ambiente de teste).
class _FakeCameraPermissionsRepository implements CameraPermissionsRepository {
  _FakeCameraPermissionsRepository({
    this.has = false,
    this.grantOnRequest = false,
    this.permanentlyDenied = false,
  });

  final bool has;
  final bool grantOnRequest;
  final bool permanentlyDenied;

  @override
  Future<bool> hasCameraPermission() async => has;

  @override
  Future<bool> requestCameraPermission() async => grantOnRequest;

  @override
  Future<bool> isPermanentlyDenied() async => permanentlyDenied;
}

void main() {
  ({ProviderContainer container, GoRouter router}) build(
    CameraPermissionsRepository repo,
  ) {
    final router = GoRouter(
      initialLocation: AppRoutes.cameraPermissions,
      routes: [
        GoRoute(
          path: AppRoutes.cameraPermissions,
          builder: (_, _) => const CameraPermissionsScreen(),
        ),
        GoRoute(
          path: AppRoutes.pskScanQr,
          builder: (_, _) => const Text('TELA DE SCANNER'),
        ),
      ],
    );
    final container = ProviderContainer(
      overrides: [cameraPermissionsRepositoryProvider.overrideWithValue(repo)],
    );
    return (container: container, router: router);
  }

  Future<void> pump(
    WidgetTester tester,
    CameraPermissionsRepository repo,
  ) async {
    final built = build(repo);
    addTearDown(built.container.dispose);
    await tester.pumpWidget(
      UncontrolledProviderScope(
        container: built.container,
        child: MaterialApp.router(routerConfig: built.router),
      ),
    );
    await tester.pumpAndSettle();
  }

  testWidgets('permissão já concedida: avança direto pro scanner', (
    tester,
  ) async {
    await pump(tester, _FakeCameraPermissionsRepository(has: true));

    expect(find.text('TELA DE SCANNER'), findsOneWidget);
    expect(find.text('Permitir câmera'), findsNothing);
  });

  testWidgets('sem permissão: pedir e conceder avança pro scanner', (
    tester,
  ) async {
    await pump(
      tester,
      _FakeCameraPermissionsRepository(grantOnRequest: true),
    );

    expect(find.text('Permitir câmera'), findsOneWidget);

    await tester.tap(find.text('Permitir'));
    await tester.pumpAndSettle();

    expect(find.text('TELA DE SCANNER'), findsOneWidget);
  });

  testWidgets('negado e travado: oferece abrir os ajustes do app', (
    tester,
  ) async {
    await pump(
      tester,
      _FakeCameraPermissionsRepository(permanentlyDenied: true),
    );

    await tester.tap(find.text('Permitir'));
    await tester.pumpAndSettle();

    expect(find.text('Abrir ajustes do app'), findsOneWidget);
    expect(find.text('Permitir'), findsNothing);
  });

  testWidgets(
    '"Digitar a chave manualmente" volta sem ir pro scanner',
    (tester) async {
      String? poppedWith = 'não-chamado';
      final built = build(_FakeCameraPermissionsRepository());
      addTearDown(built.container.dispose);
      final router = GoRouter(
        initialLocation: '/root',
        routes: [
          GoRoute(
            path: '/root',
            builder: (context, state) => Scaffold(
              body: Center(
                child: TextButton(
                  onPressed: () async {
                    poppedWith = await context.push<String>(
                      AppRoutes.cameraPermissions,
                    );
                  },
                  child: const Text('abrir permissão'),
                ),
              ),
            ),
          ),
          GoRoute(
            path: AppRoutes.cameraPermissions,
            builder: (_, _) => const CameraPermissionsScreen(),
          ),
          GoRoute(
            path: AppRoutes.pskScanQr,
            builder: (_, _) => const Text('TELA DE SCANNER'),
          ),
        ],
      );

      await tester.pumpWidget(
        UncontrolledProviderScope(
          container: built.container,
          child: MaterialApp.router(routerConfig: router),
        ),
      );

      await tester.tap(find.text('abrir permissão'));
      await tester.pumpAndSettle();
      expect(find.text('Permitir câmera'), findsOneWidget);

      await tester.tap(find.text('Digitar a chave manualmente'));
      await tester.pumpAndSettle();

      expect(find.text('abrir permissão'), findsOneWidget);
      expect(poppedWith, isNull);
    },
  );
}
