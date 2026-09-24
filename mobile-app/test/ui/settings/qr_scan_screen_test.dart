import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:go_router/go_router.dart';
import 'package:tccelta_mobile/src/domain/repositories/camera_permissions_repository.dart';
import 'package:tccelta_mobile/src/ui/settings/settings_providers.dart';
import 'package:tccelta_mobile/src/ui/settings/view/qr_scan_screen.dart';

/// Repository fake controlável (sem tocar no plugin real).
///
/// Só exercita os ramos de permissão (`idle`/`denied`/`permanentlyDenied`) —
/// o ramo `granted` (câmera ao vivo, `MobileScanner`) não é coberto por
/// widget test aqui: montar o preview real tocaria o canal de plataforma do
/// `mobile_scanner`, que não existe no ambiente de teste. Esse ramo é
/// exercitado pela lógica pura em `qr_scan_view_model_test.dart`.
class _FakeCameraPermissionsRepository implements CameraPermissionsRepository {
  _FakeCameraPermissionsRepository({this.permanentlyDenied = false});

  final bool permanentlyDenied;

  @override
  Future<bool> hasCameraPermission() async => false;

  @override
  Future<bool> requestCameraPermission() async => false;

  @override
  Future<bool> isPermanentlyDenied() async => permanentlyDenied;
}

void main() {
  Future<void> pumpScreen(
    WidgetTester tester,
    CameraPermissionsRepository repo,
  ) async {
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          cameraPermissionsRepositoryProvider.overrideWithValue(repo),
        ],
        child: const MaterialApp(home: QrScanScreen()),
      ),
    );
    await tester.pumpAndSettle();
  }

  testWidgets('sem permissão: mostra pedido de permissão, não a câmera', (
    tester,
  ) async {
    await pumpScreen(tester, _FakeCameraPermissionsRepository());

    expect(find.text('Permitir câmera'), findsOneWidget);
    expect(find.text('Permitir'), findsOneWidget);
    expect(find.text('Abrir ajustes do app'), findsNothing);
  });

  testWidgets('permissão negada e travada: oferece abrir os ajustes do app', (
    tester,
  ) async {
    await pumpScreen(
      tester,
      _FakeCameraPermissionsRepository(permanentlyDenied: true),
    );

    await tester.tap(find.text('Permitir'));
    await tester.pumpAndSettle();

    expect(find.text('Abrir ajustes do app'), findsOneWidget);
    expect(find.text('Permitir'), findsNothing);
  });

  testWidgets(
    '"Digitar a chave manualmente" fecha a tela sem resultado',
    (tester) async {
      String? poppedWith = 'não-chamado';
      final router = GoRouter(
        initialLocation: '/root',
        routes: [
          GoRoute(
            path: '/root',
            builder: (context, state) => Scaffold(
              body: Center(
                child: TextButton(
                  onPressed: () async {
                    poppedWith = await context.push<String>('/scan-qr');
                  },
                  child: const Text('abrir scanner'),
                ),
              ),
            ),
          ),
          GoRoute(
            path: '/scan-qr',
            builder: (context, state) => const QrScanScreen(),
          ),
        ],
      );

      await tester.pumpWidget(
        ProviderScope(
          overrides: [
            cameraPermissionsRepositoryProvider.overrideWithValue(
              _FakeCameraPermissionsRepository(),
            ),
          ],
          child: MaterialApp.router(routerConfig: router),
        ),
      );

      await tester.tap(find.text('abrir scanner'));
      await tester.pumpAndSettle();
      expect(find.text('Permitir câmera'), findsOneWidget);

      await tester.tap(find.text('Digitar a chave manualmente'));
      await tester.pumpAndSettle();

      expect(find.text('abrir scanner'), findsOneWidget);
      expect(poppedWith, isNull);
    },
  );
}
