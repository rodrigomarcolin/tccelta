import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/domain/repositories/camera_permissions_repository.dart';
import 'package:tccelta_mobile/src/ui/settings/settings_providers.dart';
import 'package:tccelta_mobile/src/ui/settings/view_model/camera_permissions_view_model.dart';

/// Repository fake controlável (sem tocar no plugin real).
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
  ProviderContainer containerWith(CameraPermissionsRepository repo) {
    final container = ProviderContainer(
      overrides: [cameraPermissionsRepositoryProvider.overrideWithValue(repo)],
    );
    addTearDown(container.dispose);
    // Mantém o provider `autoDispose` vivo durante o teste (sem um listener
    // ele seria descartado entre `read`s, reiniciando o `_check`).
    container.listen<CameraPermissionFlowState>(
      cameraPermissionsViewModelProvider,
      (_, _) {},
      fireImmediately: true,
    );
    return container;
  }

  test('build inicia em checking', () {
    final container = containerWith(_FakeCameraPermissionsRepository());
    expect(
      container.read(cameraPermissionsViewModelProvider),
      CameraPermissionFlowState.checking,
    );
  });

  test('permissão já concedida => converge para granted', () async {
    final container = containerWith(
      _FakeCameraPermissionsRepository(has: true),
    );
    await container.pump();
    expect(
      container.read(cameraPermissionsViewModelProvider),
      CameraPermissionFlowState.granted,
    );
  });

  test('sem permissão => idle e, após request concedido, granted', () async {
    final container = containerWith(
      _FakeCameraPermissionsRepository(grantOnRequest: true),
    );
    await container.pump();
    expect(
      container.read(cameraPermissionsViewModelProvider),
      CameraPermissionFlowState.idle,
    );

    final ok = await container
        .read(cameraPermissionsViewModelProvider.notifier)
        .request();
    expect(ok, isTrue);
    expect(
      container.read(cameraPermissionsViewModelProvider),
      CameraPermissionFlowState.granted,
    );
  });

  test('request negado, não travado => denied', () async {
    final container = containerWith(_FakeCameraPermissionsRepository());
    await container.pump();

    final ok = await container
        .read(cameraPermissionsViewModelProvider.notifier)
        .request();
    expect(ok, isFalse);
    expect(
      container.read(cameraPermissionsViewModelProvider),
      CameraPermissionFlowState.denied,
    );
  });

  test('request negado, travado => permanentlyDenied', () async {
    final container = containerWith(
      _FakeCameraPermissionsRepository(permanentlyDenied: true),
    );
    await container.pump();

    final ok = await container
        .read(cameraPermissionsViewModelProvider.notifier)
        .request();
    expect(ok, isFalse);
    expect(
      container.read(cameraPermissionsViewModelProvider),
      CameraPermissionFlowState.permanentlyDenied,
    );
  });
}
