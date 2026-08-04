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
      await container.read(scanViewModelProvider.notifier).startScan();
      await Future<void>.delayed(const Duration(milliseconds: 10));

      final state = container.read(scanViewModelProvider);
      expect(state.devices, const [device]);
      expect(state.isScanning, isFalse);
      expect(state.failure, isNull);
    });

    test('toques rápidos não reiniciam o scan em andamento (coalesce)',
        () async {
      final fake = FakeBleService(holdScanOpen: true, devices: const [device]);
      final container = makeContainer(fake);
      final vm = container.read(scanViewModelProvider.notifier);
      await vm.startScan();
      await Future<void>.delayed(const Duration(milliseconds: 10));
      expect(container.read(scanViewModelProvider).devices, const [device]);
      expect(fake.scanCount, 1);

      // Rajada de toques enquanto o scan segue ativo: são ignorados, então o
      // scan não é reiniciado (nada de churn start/stop -> sem throttle) e o
      // dongle continua na lista.
      await Future.wait<void>([vm.startScan(), vm.startScan(), vm.startScan()]);
      await Future<void>.delayed(const Duration(milliseconds: 10));

      expect(fake.scanCount, 1);
      expect(container.read(scanViewModelProvider).devices, const [device]);
    });

    test('erro no scan vira Failure no estado', () async {
      final container =
          makeContainer(FakeBleService(scanError: Exception('x')));
      await container.read(scanViewModelProvider.notifier).startScan();
      await Future<void>.delayed(const Duration(milliseconds: 10));

      final state = container.read(scanViewModelProvider);
      expect(state.failure, isA<BleScanFailure>());
      expect(state.isScanning, isFalse);
    });
  });
}
