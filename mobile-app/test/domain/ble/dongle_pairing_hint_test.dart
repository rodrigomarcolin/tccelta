import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/domain/ble/dongle_pairing_hint.dart';

void main() {
  DonglePairingHint? parse(List<int>? bytes) =>
      DonglePairingHint.fromManufacturerData({
        0xFFFF: ?bytes,
      });

  test('bit 0 = janela aberta', () {
    final hint = parse([0x01, 0x01]);
    expect(hint?.windowOpen, isTrue);
    expect(hint?.hasBond, isFalse);
  });

  test('bit 1 = há bond', () {
    final hint = parse([0x01, 0x02]);
    expect(hint?.windowOpen, isFalse);
    expect(hint?.hasBond, isTrue);
  });

  test('ambos os bits', () {
    final hint = parse([0x01, 0x03]);
    expect(hint?.windowOpen, isTrue);
    expect(hint?.hasBond, isTrue);
  });

  test('sem o Company ID, com outra versão ou curto: null', () {
    expect(parse(null), isNull);
    expect(parse([0x02, 0x01]), isNull);
    expect(parse([0x01]), isNull);
  });
}
