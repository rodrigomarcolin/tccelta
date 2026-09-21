import 'dart:convert' show utf8;
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/core/crypto/hkdf.dart';

Uint8List _fromHex(String hex) {
  final clean = hex.replaceAll(' ', '');
  final out = Uint8List(clean.length ~/ 2);
  for (var i = 0; i < out.length; i++) {
    out[i] = int.parse(clean.substring(i * 2, i * 2 + 2), radix: 16);
  }
  return out;
}

String _toHex(Uint8List bytes) {
  final buf = StringBuffer();
  for (final b in bytes) {
    buf.write(b.toRadixString(16).padLeft(2, '0'));
  }
  return buf.toString();
}

void main() {
  group('CryptoUtils.constantTimeEquals', () {
    test('returns true for identical byte lists', () {
      final a = [1, 2, 3, 4, 5];
      final b = [1, 2, 3, 4, 5];
      expect(CryptoUtils.constantTimeEquals(a, b), isTrue);
    });

    test('returns false for different lengths', () {
      final a = [1, 2, 3];
      final b = [1, 2, 3, 4];
      expect(CryptoUtils.constantTimeEquals(a, b), isFalse);
    });

    test('returns false for byte mismatches', () {
      final a = [1, 2, 3, 4];
      final b = [1, 2, 99, 4];
      expect(CryptoUtils.constantTimeEquals(a, b), isFalse);
    });

    test('returns true for empty lists', () {
      expect(CryptoUtils.constantTimeEquals([], []), isTrue);
    });
  });

  group('CryptoUtils.zeroize', () {
    test('clears byte buffer in-place', () {
      final bytes = Uint8List.fromList([0xAA, 0xBB, 0xCC, 0xDD]);
      CryptoUtils.zeroize(bytes);
      expect(bytes, equals(Uint8List(4)));
    });
  });

  group('CryptoUtils.hmacSha256', () {
    test('computes correct HMAC for known vector', () {
      // RFC 4231 Test Case 2 (Key = "Jefe", Data = "what do ya want for nothing?")
      final key = Uint8List.fromList(utf8.encode('Jefe'));
      final data = Uint8List.fromList(
        utf8.encode('what do ya want for nothing?'),
      );
      const expected =
          '5bdcc146bf60754e6a042426089575c75a003f089d2739839dec58b964ec3843';

      final mac = CryptoUtils.hmacSha256(key, data);
      expect(_toHex(mac), equals(expected));
    });
  });

  group('CryptoUtils.hkdfSha256', () {
    test('RFC 5869 Test Case 1', () {
      final ikm = _fromHex('0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b');
      final salt = _fromHex('000102030405060708090a0b0c');
      final info = _fromHex('f0f1f2f3f4f5f6f7f8f9');
      const length = 42;
      const expectedOkm =
          '3cb25f25faacd57a90434f64d0362f2a'
          '2d2d0a90cf1a5a4c5db02d56ecc4c5bf'
          '34007208d5b887185865';

      final okm = CryptoUtils.hkdfSha256(
        ikm: ikm,
        salt: salt,
        info: info,
        length: length,
      );

      expect(_toHex(okm), equals(expectedOkm));
    });

    test('RFC 5869 Test Case 2 (length > 32 bytes)', () {
      final ikm = _fromHex(
        '000102030405060708090a0b0c0d0e0f'
        '101112131415161718191a1b1c1d1e1f'
        '202122232425262728292a2b2c2d2e2f'
        '303132333435363738393a3b3c3d3e3f'
        '404142434445464748494a4b4c4d4e4f',
      );
      final salt = _fromHex(
        '606162636465666768696a6b6c6d6e6f'
        '707172737475767778797a7b7c7d7e7f'
        '808182838485868788898a8b8c8d8e8f'
        '909192939495969798999a9b9c9d9e9f'
        'a0a1a2a3a4a5a6a7a8a9aaabacadaeaf',
      );
      final info = _fromHex(
        'b0b1b2b3b4b5b6b7b8b9babbbcbdbebf'
        'c0c1c2c3c4c5c6c7c8c9cacbcccdcecf'
        'd0d1d2d3d4d5d6d7d8d9dadbdcdddedf'
        'e0e1e2e3e4e5e6e7e8e9eaebecedeeef'
        'f0f1f2f3f4f5f6f7f8f9fafbfcfdfeff',
      );
      const length = 82;
      const expectedOkm =
          'b11e398dc80327a1c8e7f78c596a4934'
          '4f012eda2d4efad8a050cc4c19afa97c'
          '59045a99cac7827271cb41c65e590e09'
          'da3275600c2f09b8367793a9aca3db71'
          'cc30c58179ec3e87c14c01d5c1f3434f'
          '1d87';

      final okm = CryptoUtils.hkdfSha256(
        ikm: ikm,
        salt: salt,
        info: info,
        length: length,
      );

      expect(_toHex(okm), equals(expectedOkm));
    });

    test('RFC 5869 Test Case 3 (Zero salt)', () {
      final ikm = _fromHex('0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b');
      final salt = Uint8List(0);
      final info = Uint8List(0);
      const length = 42;
      const expectedOkm =
          '8da4e775a563c18f715f802a063c5a31'
          'b8a11f5c5ee1879ec3454e5f3c738d2d'
          '9d201395faa4b61a96c8';

      final okm = CryptoUtils.hkdfSha256(
        ikm: ikm,
        salt: salt,
        info: info,
        length: length,
      );

      expect(_toHex(okm), equals(expectedOkm));
    });
  });
}
