import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/domain/obd2/monitor_status.dart';

void main() {
  group('monitorStatusMilOn', () {
    test('bit 7 do byte A ligado -> MIL aceso', () {
      expect(monitorStatusMilOn([0x82, 0x00, 0x00, 0x00]), isTrue);
    });

    test('bit 7 do byte A desligado -> MIL apagado', () {
      expect(monitorStatusMilOn([0x02, 0x00, 0x00, 0x00]), isFalse);
    });

    test('payload vazio -> MIL apagado', () {
      expect(monitorStatusMilOn(const []), isFalse);
    });
  });
}
