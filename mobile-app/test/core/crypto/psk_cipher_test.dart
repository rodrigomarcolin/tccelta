import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/core/crypto/psk_cipher.dart';

void main() {
  // A fixed 32-byte key (64 hex chars) for deterministic tests.
  const testKeyHex =
      'aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899';

  group('PskCipher.fromHex', () {
    test('parses a valid 64-char hex key', () {
      expect(() => PskCipher.fromHex(testKeyHex), returnsNormally);
    });

    test('throws on short hex string', () {
      expect(
        () => PskCipher.fromHex('aabbcc'),
        throwsA(isA<ArgumentError>()),
      );
    });

    test('throws on non-hex chars', () {
      // Replace last two chars with non-hex 'ZZ' to trigger parse failure.
      final bad = '${testKeyHex.substring(0, 62)}ZZ';
      expect(
        () => PskCipher.fromHex(bad),
        throwsA(isA<FormatException>()),
      );
    });
  });

  group('PskCipher encrypt/decrypt', () {
    late PskCipher cipher;

    setUp(() => cipher = PskCipher.fromHex(testKeyHex));

    test('round-trip: decrypt(encrypt(plain)) == plain', () {
      final plain = Uint8List.fromList('ATZ\r'.codeUnits);
      final frame = cipher.encrypt(plain);
      final result = cipher.decrypt(frame);

      expect(result, isNotNull);
      expect(result, equals(plain));
    });

    test('round-trip with empty plaintext', () {
      final frame = cipher.encrypt(Uint8List(0));
      final result = cipher.decrypt(frame);
      expect(result, isNotNull);
      expect(result!.isEmpty, isTrue);
    });

    test('output length: IV(12) + CT(n) + TAG(16)', () {
      final plain = Uint8List.fromList(List.generate(20, (i) => i));
      final frame = cipher.encrypt(plain);
      expect(frame.length, equals(12 + 20 + 16));
    });

    test('different IVs produce different ciphertexts for same plaintext', () {
      final plain = Uint8List.fromList('010D\r'.codeUnits);
      final frame1 = cipher.encrypt(plain);
      final frame2 = cipher.encrypt(plain);
      // IV is random per call, so frames should differ (with overwhelming probability).
      expect(frame1, isNot(equals(frame2)));
    });

    test('decrypt with wrong key returns null (auth failure)', () {
      final plain = Uint8List.fromList('hello'.codeUnits);
      final frame = cipher.encrypt(plain);

      const wrongKeyHex =
          '0011223344556677889900aabbccddeeff001122334455667788990011aabb00';
      final wrongCipher = PskCipher.fromHex(wrongKeyHex);
      expect(wrongCipher.decrypt(frame), isNull);
    });

    test('decrypt with truncated frame returns null (too short)', () {
      expect(cipher.decrypt(Uint8List(5)), isNull);
    });

    test('decrypt with bitflip in ciphertext returns null (auth failure)', () {
      final plain = Uint8List.fromList('test data'.codeUnits);
      final frame = cipher.encrypt(plain);
      // Flip a bit in the ciphertext body (after 12-byte IV, before 16-byte tag).
      frame[12] ^= 0xFF;
      expect(cipher.decrypt(frame), isNull);
    });

    test('decrypt with bitflip in tag returns null (auth failure)', () {
      final plain = Uint8List.fromList('test data'.codeUnits);
      final frame = cipher.encrypt(plain);
      // Flip a bit in the last (TAG) byte.
      frame[frame.length - 1] ^= 0x01;
      expect(cipher.decrypt(frame), isNull);
    });
  });
}
