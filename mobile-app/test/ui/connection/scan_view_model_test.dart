import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/core/errors/ble_failure.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_device.dart';
import 'package:tccelta_mobile/src/ui/connection/connection_providers.dart';
import 'package:tccelta_mobile/src/ui/connection/view_model/scan_view_model.dart';

import '../../support/fake_ble_service.dart';

void main() {
  group('ScanViewModel', () {
    const device = BleDevice(id: '1', name: 'OBD2Dongle', rssi: -50);

    ProviderContainer makeContainer(FakeBleService fake) {
      final container = ProviderContainer(
        overrides: [bleServiceProvider.overrideWithValue(fake)],
      );
      addTearDown(container.dispose);
      return container;
    }

    test('startScan popula devices e encerra o scanning', () async {
      final container = makeContainer(FakeBleService(devices: const [device]));
      container.read(scanViewModelProvider.notifier).startScan();
      await Future<void>.delayed(const Duration(milliseconds: 10));

      final state = container.read(scanViewModelProvider);
      expect(state.devices, const [device]);
      expect(state.isScanning, isFalse);
      expect(state.failure, isNull);
    });

    test('erro no scan vira Failure no estado', () async {
      final container =
          makeContainer(FakeBleService(scanError: Exception('x')));
      container.read(scanViewModelProvider.notifier).startScan();
      await Future<void>.delayed(const Duration(milliseconds: 10));

      final state = container.read(scanViewModelProvider);
      expect(state.failure, isA<BleScanFailure>());
      expect(state.isScanning, isFalse);
    });
  });
}
