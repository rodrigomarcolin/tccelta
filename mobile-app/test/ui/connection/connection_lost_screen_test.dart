import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:go_router/go_router.dart';
import 'package:tccelta_mobile/src/application/connection/forget_dongle_use_case.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_link_error.dart';
import 'package:tccelta_mobile/src/router/app_routes.dart';
import 'package:tccelta_mobile/src/ui/connection/connection_providers.dart';
import 'package:tccelta_mobile/src/ui/connection/view/connection_lost_screen.dart';
import 'package:tccelta_mobile/src/ui/connection/view_model/connecting_view_model.dart';

import '../../support/fake_connecting_view_model.dart';

/// [ForgetDongleUseCase] falso que só conta as chamadas.
class _FakeForgetUseCase implements ForgetDongleUseCase {
  int calls = 0;

  @override
  Future<ForgetDongleResult> call() async {
    calls++;
    return const ForgetDongleResult(
      bondRemoved: true,
      needsManualForget: false,
    );
  }
}

void main() {
  late FakeConnectingViewModel fake;
  late _FakeForgetUseCase forget;

  Future<void> pumpScreen(WidgetTester tester, ConnectingState state) async {
    fake = FakeConnectingViewModel(state);
    forget = _FakeForgetUseCase();
    Widget stub(String name) => Scaffold(body: Text('rota:$name'));
    final router = GoRouter(
      initialLocation: AppRoutes.connectionLost,
      routes: [
        GoRoute(
          path: AppRoutes.connectionLost,
          builder: (_, _) => const ConnectionLostScreen(),
        ),
        GoRoute(
          path: AppRoutes.connecting,
          builder: (_, _) => stub('connecting'),
        ),
        GoRoute(path: AppRoutes.pairing, builder: (_, _) => stub('pairing')),
        GoRoute(
          path: AppRoutes.blePermissions,
          builder: (_, _) => stub('permissions'),
        ),
      ],
    );
    tester.view.physicalSize = const Size(800, 2400);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          connectingViewModelProvider.overrideWith(() => fake),
          forgetDongleUseCaseProvider.overrideWithValue(forget),
        ],
        child: MaterialApp.router(theme: AppTheme.dark, routerConfig: router),
      ),
    );
    await tester.pump();
  }

  group('queda comum', () {
    testWidgets('sem erro classificado: mensagem de conexão perdida', (
      tester,
    ) async {
      await pumpScreen(tester, const ConnectingState());

      expect(find.text('Conexão perdida'), findsOneWidget);
      expect(find.text('Reconectar'), findsOneWidget);
      expect(find.text('Parear novamente'), findsNothing);
    });

    for (final error in [
      BleLinkError.timeout,
      BleLinkError.busy,
      BleLinkError.other,
    ]) {
      testWidgets('$error não é perda de pareamento', (tester) async {
        await pumpScreen(tester, ConnectingState(linkError: error));

        expect(find.text('Conexão perdida'), findsOneWidget);
        expect(find.text('Reconectar'), findsOneWidget);
      });
    }

    testWidgets('Reconectar volta para a tela de conexão', (tester) async {
      await pumpScreen(tester, const ConnectingState());

      await tester.tap(find.text('Reconectar'));
      await tester.pumpAndSettle();

      expect(find.text('rota:connecting'), findsOneWidget);
      expect(fake.calls, isEmpty);
    });
  });

  group('pareamento perdido', () {
    for (final error in [
      BleLinkError.bondLost,
      BleLinkError.bondLostIos,
      BleLinkError.notAuthorized,
      BleLinkError.remoteReset,
    ]) {
      testWidgets('$error mostra "Parear novamente"', (tester) async {
        await pumpScreen(tester, ConnectingState(linkError: error));

        expect(
          find.text('O dongle não reconhece mais este aparelho'),
          findsOneWidget,
        );
        expect(find.text('Parear novamente'), findsOneWidget);
        expect(find.text('Reconectar'), findsNothing);
      });
    }

    testWidgets('Android: repair() e segue para a conexão', (tester) async {
      await pumpScreen(
        tester,
        const ConnectingState(linkError: BleLinkError.bondLost),
      );
      fake.afterRepair = const ConnectingState(
        linkError: BleLinkError.bondLost,
      );

      await tester.tap(find.text('Parear novamente'));
      await tester.pumpAndSettle();

      expect(fake.calls, ['repair']);
      expect(find.text('rota:connecting'), findsOneWidget);
    });

    testWidgets('iOS: repair() orienta na tela de pareamento', (tester) async {
      await pumpScreen(
        tester,
        const ConnectingState(
          linkError: BleLinkError.bondLostIos,
          canManageBonds: false,
        ),
      );
      fake.afterRepair = const ConnectingState(
        linkError: BleLinkError.bondLostIos,
        security: LinkSecurityStep.forgetOnIos,
        canManageBonds: false,
      );

      await tester.tap(find.text('Parear novamente'));
      await tester.pumpAndSettle();

      expect(fake.calls, ['repair']);
      expect(find.text('rota:pairing'), findsOneWidget);
    });
  });

  group('Esquecer dispositivo', () {
    testWidgets('esquece o dongle e vai para as permissões', (tester) async {
      await pumpScreen(tester, const ConnectingState());

      await tester.tap(find.text('Esquecer dispositivo'));
      await tester.pumpAndSettle();

      expect(forget.calls, 1);
      expect(find.text('rota:permissions'), findsOneWidget);
    });

    testWidgets('também está disponível quando o pareamento foi perdido', (
      tester,
    ) async {
      await pumpScreen(
        tester,
        const ConnectingState(linkError: BleLinkError.remoteReset),
      );

      await tester.tap(find.text('Esquecer dispositivo'));
      await tester.pumpAndSettle();

      expect(forget.calls, 1);
      expect(find.text('rota:permissions'), findsOneWidget);
    });
  });
}
