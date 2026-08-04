import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_signal_level.dart';

void main() {
  group('BleSignalLevel.fromRssi', () {
    test('forte quando >= -60 dBm', () {
      expect(BleSignalLevel.fromRssi(-30), BleSignalLevel.strong);
      expect(BleSignalLevel.fromRssi(-60), BleSignalLevel.strong);
    });

    test('médio na faixa [-75, -60)', () {
      expect(BleSignalLevel.fromRssi(-61), BleSignalLevel.medium);
      expect(BleSignalLevel.fromRssi(-75), BleSignalLevel.medium);
    });

    test('fraco quando < -75 dBm', () {
      expect(BleSignalLevel.fromRssi(-76), BleSignalLevel.weak);
      expect(BleSignalLevel.fromRssi(-100), BleSignalLevel.weak);
    });
  });
}
