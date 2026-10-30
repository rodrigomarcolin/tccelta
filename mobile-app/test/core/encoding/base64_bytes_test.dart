import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/core/encoding/base64_bytes.dart';

void main() {
  test('ida e volta com padding', () {
    final bytes = Uint8List.fromList([0xFB, 0xFF, 0xFE, 1, 2]);
    final text = Base64Bytes.encode(bytes);
    expect(text, '+//+AQI=');
    expect(Base64Bytes.decode(text), bytes);
  });

  test('rejeita URL-safe e sem padding', () {
    expect(() => Base64Bytes.decode('-__-AQI='), throwsFormatException);
    expect(() => Base64Bytes.decode('+//+AQI'), throwsFormatException);
    expect(() => Base64Bytes.decode('ab=c'), throwsFormatException);
  });
}
