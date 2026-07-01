import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/core/errors/ble_failure.dart';
import 'package:tccelta_mobile/src/data/datasources/dongle_datasource.dart';
import 'package:tccelta_mobile/src/data/repositories/dongle_repository_impl.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_device.dart';

import '../../support/fake_ble_service.dart';

void main() {
  group('DongleRepositoryImpl', () {
    const device = BleDevice(id: '1', name: 'OBD2Dongle', rssi: -50);

    DongleRepositoryImpl makeRepo(FakeBleService fake) =>
        DongleRepositoryImpl(DongleDatasource(fake));

    test('scan emite os dongles vistos', () async {
      final repo = makeRepo(FakeBleService(devices: const [device]));
      expect(await repo.scan().first, const [device]);
    });

    test('scan mapeia erro cru para BleScanFailure', () async {
      final repo = makeRepo(FakeBleService(scanError: Exception('boom')));
      await expectLater(repo.scan(), emitsError(isA<BleScanFailure>()));
    });

    test('connect chega a ready e expõe a conexão', () async {
      final repo = makeRepo(FakeBleService());
      await repo.connect('1');
      await expectLater(
        repo.connectionPhase,
        emitsThrough(BleConnectionPhase.ready),
      );
      expect(repo.isReady, isTrue);
      expect(repo.connection, isNotNull);
    });
  });
}
