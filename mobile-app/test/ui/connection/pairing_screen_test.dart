import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:go_router/go_router.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';
import 'package:tccelta_mobile/src/router/app_routes.dart';
import 'package:tccelta_mobile/src/ui/connection/view/pairing_screen.dart';
import 'package:tccelta_mobile/src/ui/connection/view_model/connecting_view_model.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';

import '../../support/fake_connecting_view_model.dart';

void main() {
  late FakeConnectingViewModel fake;

  Widget wrap(ConnectingState initial) {
    fake = FakeConnectingViewModel(initial);
    Widget stub(String name) => Scaffold(body: Text('rota:$name'));
    final router = GoRouter(
      initialLocation: AppRoutes.pairing,
      routes: [
        GoRoute(
          path: AppRoutes.pairing,
          builder: (_, _) => const PairingScreen(),
        ),
        GoRoute(
          path: AppRoutes.connecting,
          builder: (_, _) => stub('connecting'),
        ),
        GoRoute(
          path: AppRoutes.connectionLost,
          builder: (_, _) => stub('lost'),
        ),
        GoRoute(path: AppRoutes.scan, builder: (_, _) => stub('scan')),
      ],
    );
    return ProviderScope(
      overrides: [connectingViewModelProvider.overrideWith(() => fake)],
      child: MaterialApp.router(theme: AppTheme.dark, routerConfig: router),
    );
  }

  Future<void> pumpScreen(WidgetTester tester, ConnectingState state) async {
    tester.view.physicalSize = const Size(800, 2400);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    await tester.pumpWidget(wrap(state));
    await tester.pump();
  }

  const waiting = ConnectingState(
    security: LinkSecurityStep.waitingButton,
    watching: true,
  );

  group('aguardando o botão', () {
    testWidgets('procurando: mostra orientação e "Cancelar"', (tester) async {
      await pumpScreen(tester, waiting);

      expect(
        find.text('Coloque o dongle em modo de pareamento'),
        findsOneWidget,
      );
      expect(find.text('PROCURANDO O DONGLE…'), findsOneWidget);
      expect(
        find.text('Com o dongle ligado, segure o botão por 5 segundos'),
        findsOneWidget,
      );
      expect(find.text('Cancelar'), findsOneWidget);
      expect(find.text('Tentar de novo'), findsNothing);
    });

    testWidgets('dongle visto: badge muda para encontrado', (tester) async {
      await pumpScreen(
        tester,
        const ConnectingState(
          security: LinkSecurityStep.waitingButton,
          watching: true,
          dongleSeen: true,
        ),
      );

      expect(
        find.text('DONGLE ENCONTRADO — AGUARDANDO O BOTÃO'),
        findsOneWidget,
      );
    });

    testWidgets('prazo esgotado: "Dongle não visto" e "Tentar de novo"', (
      tester,
    ) async {
      await pumpScreen(
        tester,
        const ConnectingState(security: LinkSecurityStep.waitingButton),
      );

      expect(find.text('DONGLE NÃO VISTO'), findsOneWidget);
      await tester.tap(find.text('Tentar de novo'));
      await tester.pump(const Duration(seconds: 1));

      expect(fake.calls, ['retry']);
    });

    testWidgets('Cancelar chama cancel() e volta para a busca', (tester) async {
      await pumpScreen(tester, waiting);

      await tester.tap(find.text('Cancelar'));
      await tester.pumpAndSettle();

      expect(fake.calls, ['cancel']);
      expect(find.text('rota:scan'), findsOneWidget);
    });

    testWidgets('iOS: oferece "Já pareei este iPhone"', (tester) async {
      await pumpScreen(
        tester,
        const ConnectingState(
          security: LinkSecurityStep.waitingButton,
          watching: true,
          canManageBonds: false,
        ),
      );

      await tester.tap(find.text('Já pareei este iPhone'));
      await tester.pump(const Duration(seconds: 1));

      expect(fake.calls, ['assumePaired']);
    });

    testWidgets('Android: não oferece "Já pareei este iPhone"', (tester) async {
      await pumpScreen(tester, waiting);

      expect(find.text('Já pareei este iPhone'), findsNothing);
    });
  });

  group('janela expirada', () {
    const expired = ConnectingState(security: LinkSecurityStep.windowExpired);

    testWidgets('mostra a mensagem e refaz a espera pelo botão', (
      tester,
    ) async {
      await pumpScreen(tester, expired);

      expect(find.text('O tempo para parear acabou'), findsOneWidget);
      await tester.tap(find.text('Segurar o botão de novo'));
      await tester.pump(const Duration(seconds: 1));

      expect(fake.calls, ['retry']);
    });

    testWidgets('Cancelar chama cancel() e volta para a busca', (tester) async {
      await pumpScreen(tester, expired);

      await tester.tap(find.text('Cancelar'));
      await tester.pumpAndSettle();

      expect(fake.calls, ['cancel']);
      expect(find.text('rota:scan'), findsOneWidget);
    });
  });

  group('pareamento recusado', () {
    const rejected = ConnectingState(security: LinkSecurityStep.rejected);

    testWidgets('mostra a mensagem e tenta de novo', (tester) async {
      await pumpScreen(tester, rejected);

      expect(find.text('Pareamento recusado'), findsOneWidget);
      await tester.tap(find.text('Tentar de novo'));
      await tester.pump(const Duration(seconds: 1));

      expect(fake.calls, ['retry']);
    });

    testWidgets('Cancelar chama cancel() e volta para a busca', (tester) async {
      await pumpScreen(tester, rejected);

      await tester.tap(find.text('Cancelar'));
      await tester.pumpAndSettle();

      expect(fake.calls, ['cancel']);
      expect(find.text('rota:scan'), findsOneWidget);
    });
  });

  group('iOS: esquecer o dispositivo', () {
    const forget = ConnectingState(
      security: LinkSecurityStep.forgetOnIos,
      canManageBonds: false,
    );

    testWidgets('lista os passos e oferece "Abrir Ajustes"', (tester) async {
      await pumpScreen(tester, forget);

      expect(
        find.text('Este iPhone guarda um pareamento antigo'),
        findsOneWidget,
      );
      expect(find.text('Abra Ajustes › Bluetooth'), findsOneWidget);
      expect(
        find.text('Toque em Esquecer este dispositivo'),
        findsOneWidget,
      );
      expect(find.text('Abrir Ajustes'), findsOneWidget);
    });

    testWidgets('"Já esqueci" chama confirmForgotten()', (tester) async {
      await pumpScreen(tester, forget);

      await tester.tap(find.text('Já esqueci'));
      await tester.pump(const Duration(seconds: 1));

      expect(fake.calls, ['confirmForgotten']);
    });
  });

  group('navegação reativa', () {
    testWidgets('segurança falha com fase failed -> conexão perdida', (
      tester,
    ) async {
      await pumpScreen(tester, waiting);

      fake.emit(
        const ConnectingState(
          security: LinkSecurityStep.failed,
          phase: BleConnectionPhase.failed,
        ),
      );
      await tester.pumpAndSettle();

      expect(find.text('rota:lost'), findsOneWidget);
    });

    testWidgets('segurança falha sem fase failed -> volta para a conexão', (
      tester,
    ) async {
      await pumpScreen(tester, waiting);

      fake.emit(const ConnectingState(security: LinkSecurityStep.failed));
      await tester.pumpAndSettle();

      expect(find.text('rota:connecting'), findsOneWidget);
    });

    for (final step in [
      LinkSecurityStep.pairing,
      LinkSecurityStep.restoring,
      LinkSecurityStep.secured,
    ]) {
      testWidgets('$step -> volta para a conexão', (tester) async {
        await pumpScreen(tester, waiting);

        fake.emit(ConnectingState(security: step));
        await tester.pumpAndSettle();

        expect(find.text('rota:connecting'), findsOneWidget);
      });
    }

    testWidgets('janela expirada (precisa de ação) -> permanece', (
      tester,
    ) async {
      await pumpScreen(tester, waiting);

      fake.emit(
        const ConnectingState(security: LinkSecurityStep.windowExpired),
      );
      await tester.pumpAndSettle();

      expect(find.byType(PairingScreen), findsOneWidget);
      expect(find.text('O tempo para parear acabou'), findsOneWidget);
    });
  });

  testWidgets('o badge pulsa só enquanto procura', (tester) async {
    await pumpScreen(tester, waiting);
    expect(tester.widget<StatusBadge>(find.byType(StatusBadge)).pulse, isTrue);

    fake.emit(const ConnectingState(security: LinkSecurityStep.waitingButton));
    await tester.pump();
    expect(tester.widget<StatusBadge>(find.byType(StatusBadge)).pulse, isFalse);
  });
}
