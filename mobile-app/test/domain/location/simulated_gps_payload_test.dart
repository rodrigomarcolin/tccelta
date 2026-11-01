import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/domain/location/simulated_gps_payload.dart';

List<int> _hex(String s) => [
  for (var i = 0; i < s.length; i += 2)
    int.parse(s.substring(i, i + 2), radix: 16),
];

void main() {
  const golden = '01010100dcf9f4f1445d25e4601c0b00';

  test('decodifica o vetor ouro', () {
    expect(
      SimulatedGpsPayload.tryDecode(_hex(golden)),
      const SimulatedGpsPayload(
        seq: 1,
        valid: true,
        latE7: -235603492,
        lonE7: -467313340,
        altMm: 728160,
      ),
    );
  });

  test('flag zerada vira posição inválida', () {
    final bytes = _hex(golden)..[1] = 0;
    expect(SimulatedGpsPayload.tryDecode(bytes)?.valid, isFalse);
  });

  test('rejeita versão diferente de 1', () {
    expect(SimulatedGpsPayload.tryDecode(_hex(golden)..[0] = 2), isNull);
    expect(SimulatedGpsPayload.tryDecode(List<int>.filled(16, 0)), isNull);
  });

  test('rejeita tamanho diferente de 16', () {
    final bytes = _hex(golden);
    expect(SimulatedGpsPayload.tryDecode(bytes.sublist(0, 15)), isNull);
    expect(SimulatedGpsPayload.tryDecode([...bytes, 0]), isNull);
    expect(SimulatedGpsPayload.tryDecode(const []), isNull);
  });

  test('rejeita latitude fora da faixa', () {
    // latE7 = 0x7FFFFFFF (> 90°).
    final bytes = _hex(golden)..setAll(4, [0xff, 0xff, 0xff, 0x7f]);
    expect(SimulatedGpsPayload.tryDecode(bytes), isNull);
  });
}
