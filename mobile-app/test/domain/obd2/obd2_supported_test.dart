import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_supported.dart';

void main() {
  group('supportedPidNumbersFromBitmap', () {
    test('decodifica o bitmask de exemplo do mock (0x181E8000)', () {
      final pids = supportedPidNumbersFromBitmap(0x00, [
        0x18,
        0x1E,
        0x80,
        0x00,
      ]);
      expect(pids, {0x04, 0x05, 0x0C, 0x0D, 0x0E, 0x0F, 0x11});
    });

    test('desloca os números pelo base do range', () {
      // Mesmo bit set (MSB do byte A) → PID base+1.
      expect(supportedPidNumbersFromBitmap(0x20, [0x80, 0x00, 0x00, 0x00]), {
        0x21,
      });
    });

    test('vazio quando vêm menos de 4 bytes', () {
      expect(supportedPidNumbersFromBitmap(0x00, [0x18, 0x1E]), isEmpty);
    });
  });

  group('bitmapHasNextRange', () {
    test('falso quando o LSB do 4º byte é 0 (mock)', () {
      expect(bitmapHasNextRange([0x18, 0x1E, 0x80, 0x00]), isFalse);
    });

    test('verdadeiro quando o LSB do 4º byte é 1', () {
      expect(bitmapHasNextRange([0x00, 0x00, 0x00, 0x01]), isTrue);
    });
  });

  group('Obd2Pid.fromByte', () {
    test('mapeia número cru → enum conhecido', () {
      expect(Obd2Pid.fromByte(0x0C), Obd2Pid.rpm);
      expect(Obd2Pid.fromByte(0x11), Obd2Pid.throttle);
    });

    test('null para PID desconhecido (ex.: 0xFF)', () {
      expect(Obd2Pid.fromByte(0xFF), isNull);
    });
  });
}
