import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:go_router/go_router.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:tccelta_mobile/src/router/app_routes.dart';
import 'package:tccelta_mobile/src/ui/settings/settings_providers.dart';
import 'package:tccelta_mobile/src/ui/settings/view/psk_settings_screen.dart';

const _validHex =
    'aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899';

/// Substitui a `QrScanScreen` real nesses testes: dois botões devolvem, via
/// `context.pop`, o resultado que a tela de origem (`PskSettingsScreen`)
/// receberia de um scan real bem-sucedido ou de um usuário que voltou sem
/// escanear.
class _StubScanScreen extends StatelessWidget {
  const _StubScanScreen();

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: Column(
        children: [
          TextButton(
            onPressed: () => context.pop(_validHex),
            child: const Text('stub: devolver hex válido'),
          ),
          TextButton(
            onPressed: () => context.pop(),
            child: const Text('stub: voltar sem escanear'),
          ),
        ],
      ),
    );
  }
}

void main() {
  setUp(() {
    SharedPreferences.setMockInitialValues({});
  });

  ({ProviderContainer container, GoRouter router}) build() {
    final router = GoRouter(
      initialLocation: AppRoutes.settings,
      routes: [
        GoRoute(
          path: AppRoutes.settings,
          builder: (_, _) => const PskSettingsScreen(),
        ),
        GoRoute(
          path: AppRoutes.pskScanQr,
          builder: (_, _) => const _StubScanScreen(),
        ),
      ],
    );
    final container = ProviderContainer();
    return (container: container, router: router);
  }

  testWidgets(
    'escanear QR e confirmar preenche o campo e salva a PSK',
    (tester) async {
      final built = build();
      addTearDown(built.container.dispose);

      await tester.pumpWidget(
        UncontrolledProviderScope(
          container: built.container,
          child: MaterialApp.router(routerConfig: built.router),
        ),
      );
      await tester.pumpAndSettle();

      await tester.tap(find.byTooltip('Escanear QR code'));
      await tester.pumpAndSettle();
      expect(find.text('stub: devolver hex válido'), findsOneWidget);

      await tester.tap(find.text('stub: devolver hex válido'));
      await tester.pumpAndSettle();

      expect(find.text(_validHex), findsOneWidget);

      await tester.tap(find.text('Continuar e salvar chave'));
      await tester.pumpAndSettle();

      expect(built.container.read(pskNotifierProvider).value, _validHex);
    },
  );

  testWidgets(
    'voltar do scanner sem escanear não altera o campo',
    (tester) async {
      final built = build();
      addTearDown(built.container.dispose);

      await tester.pumpWidget(
        UncontrolledProviderScope(
          container: built.container,
          child: MaterialApp.router(routerConfig: built.router),
        ),
      );
      await tester.pumpAndSettle();

      await tester.tap(find.byTooltip('Escanear QR code'));
      await tester.pumpAndSettle();

      await tester.tap(find.text('stub: voltar sem escanear'));
      await tester.pumpAndSettle();

      expect(find.text(_validHex), findsNothing);
      expect(built.container.read(pskNotifierProvider).value, isNull);
    },
  );
}
