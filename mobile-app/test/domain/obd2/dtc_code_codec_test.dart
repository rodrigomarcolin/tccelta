import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_code_codec.dart';

void main() {
  group('dtcCodeFromRaw', () {
    // Os 5 DTCs que o simulador/hardware (ECUSim) de fato emite.
    test('decodifica os códigos reais do simulador', () {
      expect(dtcCodeFromRaw(0x0301), 'P0301');
      expect(dtcCodeFromRaw(0x0171), 'P0171');
      expect(dtcCodeFromRaw(0x0133), 'P0133');
      expect(dtcCodeFromRaw(0x0420), 'P0420');
      expect(dtcCodeFromRaw(0x0700), 'P0700');
    });

    test('decodifica um código de cada letra (P/C/B/U)', () {
      expect(dtcCodeFromRaw(0x0121), 'P0121'); // 00 xx -> P
      expect(dtcCodeFromRaw(0x4035), 'C0035'); // 01 xx -> C
      expect(dtcCodeFromRaw(0x8200), 'B0200'); // 10 xx -> B
      expect(dtcCodeFromRaw(0xC155), 'U0155'); // 11 xx -> U
    });

    test('respeita o primeiro dígito codificado nos bits 12-13', () {
      // 0x1301 -> 00 01 3 0x301 -> "P1301".
      expect(dtcCodeFromRaw(0x1301), 'P1301');
      // 0x2301 -> 00 10 -> primeiro dígito 2.
      expect(dtcCodeFromRaw(0x2301), 'P2301');
      // 0x3301 -> 00 11 -> primeiro dígito 3.
      expect(dtcCodeFromRaw(0x3301), 'P3301');
    });
  });
}
