import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:go_router/go_router.dart';
import 'package:tccelta_mobile/src/domain/ble/last_dongle.dart';
import 'package:tccelta_mobile/src/domain/repositories/last_dongle_repository.dart';
import 'package:tccelta_mobile/src/domain/repositories/permissions_repository.dart';
import 'package:tccelta_mobile/src/router/app_routes.dart';
import 'package:tccelta_mobile/src/ui/connection/connection_providers.dart';
import 'package:tccelta_mobile/src/ui/connection/view/permissions_screen.dart';

import '../../support/fake_ble_service.dart';

/// Permissão sempre concedida: converge direto para `granted` sem UI.
class _AlwaysGrantedPermissionsRepository implements PermissionsRepository {
  @override
  Future<bool> hasBluetoothPermission() async => true;

  @override
  Future<bool> requestBluetoothPermission() async => true;
}

/// Repository fake controlável do último dongle salvo.
class _FakeLastDongleRepository implements LastDongleRepository {
  _FakeLastDongleRepository([this._saved]);

  final LastDongle? _saved;

  @override
  Future<LastDongle?> load() async => _saved;

  @override
  Future<void> save(LastDongle device) async {}

  @override
  Future<void> clear() async {}
}

void main() {
  ({ProviderContainer container, GoRouter router}) build({
    required LastDongle? saved,
  }) {
    final router = GoRouter(
      initialLocation: AppRoutes.permissions,
      routes: [
        GoRoute(
          path: AppRoutes.permissions,
          builder: (_, _) => const PermissionsScreen(),
        ),
        GoRoute(
          path: AppRoutes.scan,
          builder: (_, _) => const Text('TELA DE BUSCA'),
        ),
        GoRoute(
          path: AppRoutes.connecting,
          builder: (_, _) => const Text('TELA DE CONEXÃO'),
        ),
        GoRoute(
          path: AppRoutes.bluetoothOff,
          builder: (_, _) => const Text('BT DESLIGADO'),
        ),
      ],
    );
    final container = ProviderContainer(
      overrides: [
        bleServiceProvider.overrideWithValue(FakeBleService()),
        permissionsRepositoryProvider.overrideWithValue(
          _AlwaysGrantedPermissionsRepository(),
        ),
        lastDongleRepositoryProvider.overrideWithValue(
          _FakeLastDongleRepository(saved),
        ),
      ],
    );
    return (container: container, router: router);
  }

  testWidgets('sem dongle salvo: avança para a busca', (tester) async {
    final built = build(saved: null);
    addTearDown(built.container.dispose);

    await tester.pumpWidget(
      UncontrolledProviderScope(
        container: built.container,
        child: MaterialApp.router(routerConfig: built.router),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('TELA DE BUSCA'), findsOneWidget);
  });

  testWidgets(
    'com dongle salvo: pula direto para Conectando com a seleção populada',
    (tester) async {
      final built = build(
        saved: const LastDongle(id: 'AA:BB', name: 'OBD2Dongle'),
      );
      addTearDown(built.container.dispose);

      await tester.pumpWidget(
        UncontrolledProviderScope(
          container: built.container,
          child: MaterialApp.router(routerConfig: built.router),
        ),
      );
      await tester.pumpAndSettle();

      expect(find.text('TELA DE CONEXÃO'), findsOneWidget);
      final selected = built.container.read(selectedDongleProvider);
      expect(selected?.id, 'AA:BB');
      expect(selected?.name, 'OBD2Dongle');
    },
  );
}
