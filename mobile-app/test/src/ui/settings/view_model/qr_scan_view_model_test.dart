import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/domain/repositories/camera_permissions_repository.dart';
import 'package:tccelta_mobile/src/ui/settings/settings_providers.dart';
import 'package:tccelta_mobile/src/ui/settings/view_model/qr_scan_view_model.dart';

const _validHex =
    'aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899';

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
    container.listen<QrScanState>(
      qrScanViewModelProvider,
      (_, _) {},
      fireImmediately: true,
    );
    return container;
  }

  test('build inicia em checking', () {
    final container = containerWith(_FakeCameraPermissionsRepository());
    expect(
      container.read(qrScanViewModelProvider).permission,
      QrPermissionPhase.checking,
    );
  });

  test('permissão já concedida => converge para granted', () async {
    final container = containerWith(
      _FakeCameraPermissionsRepository(has: true),
    );
    await container.pump();
    expect(
      container.read(qrScanViewModelProvider).permission,
      QrPermissionPhase.granted,
    );
  });

  test('sem permissão => idle e, após request concedido, granted', () async {
    final container = containerWith(
      _FakeCameraPermissionsRepository(grantOnRequest: true),
    );
    await container.pump();
    expect(
      container.read(qrScanViewModelProvider).permission,
      QrPermissionPhase.idle,
    );

    await container.read(qrScanViewModelProvider.notifier).request();
    expect(
      container.read(qrScanViewModelProvider).permission,
      QrPermissionPhase.granted,
    );
  });

  test('request negado, não travado => denied', () async {
    final container = containerWith(_FakeCameraPermissionsRepository());
    await container.pump();

    await container.read(qrScanViewModelProvider.notifier).request();
    expect(
      container.read(qrScanViewModelProvider).permission,
      QrPermissionPhase.denied,
    );
  });

  test('request negado, travado => permanentlyDenied', () async {
    final container = containerWith(
      _FakeCameraPermissionsRepository(permanentlyDenied: true),
    );
    await container.pump();

    await container.read(qrScanViewModelProvider.notifier).request();
    expect(
      container.read(qrScanViewModelProvider).permission,
      QrPermissionPhase.permanentlyDenied,
    );
  });

  test('onDetected com hex válido preenche resultHex', () async {
    final container = containerWith(
      _FakeCameraPermissionsRepository(has: true),
    );
    await container.pump();

    container.read(qrScanViewModelProvider.notifier).onDetected(_validHex);
    expect(container.read(qrScanViewModelProvider).resultHex, _validHex);
  });

  test(
    'onDetected com lixo seta errorMessage e não preenche resultHex',
    () async {
      final container = containerWith(
        _FakeCameraPermissionsRepository(has: true),
      );
      await container.pump();

      container.read(qrScanViewModelProvider.notifier).onDetected('lixo');
      final state = container.read(qrScanViewModelProvider);
      expect(state.resultHex, isNull);
      expect(state.errorMessage, isNotNull);
    },
  );

  test('onDetected após já resolvido ignora frames adicionais', () async {
    final container = containerWith(
      _FakeCameraPermissionsRepository(has: true),
    );
    await container.pump();

    final notifier = container.read(qrScanViewModelProvider.notifier)
      ..onDetected(_validHex);
    final resolvedHex = _validHex.replaceFirst('a', 'b');
    notifier.onDetected(resolvedHex);

    expect(container.read(qrScanViewModelProvider).resultHex, _validHex);
  });
}
