import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/domain/repositories/permissions_repository.dart';
import 'package:tccelta_mobile/src/ui/connection/connection_providers.dart';
import 'package:tccelta_mobile/src/ui/connection/view_model/permissions_view_model.dart';

/// Repository fake controlável (sem tocar no plugin real).
class _FakePermissionsRepository implements PermissionsRepository {
  _FakePermissionsRepository({this.has = false, this.grantOnRequest = false});

  final bool has;
  final bool grantOnRequest;

  @override
  Future<bool> hasBluetoothPermission() async => has;

  @override
  Future<bool> requestBluetoothPermission() async => grantOnRequest;
}

void main() {
  ProviderContainer containerWith(PermissionsRepository repo) {
    final container = ProviderContainer(
      overrides: [permissionsRepositoryProvider.overrideWithValue(repo)],
    );
    addTearDown(container.dispose);
    return container;
  }

  test('build inicia em checking', () {
    final container = containerWith(_FakePermissionsRepository());
    expect(
      container.read(permissionsViewModelProvider),
      PermissionFlowState.checking,
    );
  });

  test('permissão já concedida => converge para granted', () async {
    final container = containerWith(_FakePermissionsRepository(has: true));
    // Dispara o build (e o _check assíncrono).
    container.read(permissionsViewModelProvider);
    await container.pump();
    expect(
      container.read(permissionsViewModelProvider),
      PermissionFlowState.granted,
    );
  });

  test('sem permissão => idle e, após request concedido, granted', () async {
    final container = containerWith(
      _FakePermissionsRepository(grantOnRequest: true),
    );
    container.read(permissionsViewModelProvider);
    await container.pump();
    expect(
      container.read(permissionsViewModelProvider),
      PermissionFlowState.idle,
    );

    final ok =
        await container.read(permissionsViewModelProvider.notifier).request();
    expect(ok, isTrue);
    expect(
      container.read(permissionsViewModelProvider),
      PermissionFlowState.granted,
    );
  });

  test('request negado => denied', () async {
    final container = containerWith(_FakePermissionsRepository());
    container.read(permissionsViewModelProvider);
    await container.pump();

    final ok =
        await container.read(permissionsViewModelProvider.notifier).request();
    expect(ok, isFalse);
    expect(
      container.read(permissionsViewModelProvider),
      PermissionFlowState.denied,
    );
  });
}
