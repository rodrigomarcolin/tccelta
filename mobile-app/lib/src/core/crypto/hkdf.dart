import 'dart:typed_data';

import 'package:pointycastle/export.dart';

/// Cryptographic utilities for HKDF-SHA256 and constant-time comparisons.
abstract final class CryptoUtils {
  /// Computes HMAC-SHA256 over [data] with [key].
  static Uint8List hmacSha256(Uint8List key, Uint8List data) {
    final hmac = HMac(SHA256Digest(), 64)..init(KeyParameter(key));
    final out = Uint8List(32);
    hmac
      ..update(data, 0, data.length)
      ..doFinal(out, 0);
    return out;
  }

  /// Extracts pseudorandom key (PRK) and expands to [length] bytes
  /// using HKDF-SHA256 (RFC 5869).
  ///
  /// - [ikm]: Input Keying Material (e.g., PSK)
  /// - [salt]: Salt value (e.g., clientNonce || serverNonce)
  /// - [info]: Context and application specific information string/bytes
  /// - [length]: Length of output keying material in bytes (typically 32)
  static Uint8List hkdfSha256({
    required Uint8List ikm,
    required Uint8List salt,
    required Uint8List info,
    int length = 32,
  }) {
    if (length <= 0 || length > 255 * 32) {
      throw ArgumentError.value(
        length,
        'length',
        'Cannot generate more than 255 * HashLen bytes',
      );
    }

    // Phase 1: Extract -> PRK = HMAC-Hash(salt, IKM)
    final effectiveSalt = salt.isEmpty ? Uint8List(32) : salt;
    final prk = hmacSha256(effectiveSalt, ikm);

    // Phase 2: Expand -> OKM
    final n = (length / 32).ceil();
    final okm = Uint8List(n * 32);
    var prev = Uint8List(0);
    var offset = 0;

    for (var i = 1; i <= n; i++) {
      final input = Uint8List(prev.length + info.length + 1);
      input
        ..setRange(0, prev.length, prev)
        ..setRange(prev.length, prev.length + info.length, info)
        ..[input.length - 1] = i;

      prev = hmacSha256(prk, input);
      okm.setRange(offset, offset + prev.length, prev);
      offset += prev.length;
    }

    // Zeroize intermediate PRK
    zeroize(prk);

    return Uint8List.sublistView(okm, 0, length);
  }

  /// Performs a constant-time comparison of two byte lists.
  /// Returns `true` if and only if both lists are identical in length and
  /// content.
  static bool constantTimeEquals(List<int> a, List<int> b) {
    if (a.length != b.length) {
      return false;
    }
    var result = 0;
    for (var i = 0; i < a.length; i++) {
      result |= a[i] ^ b[i];
    }
    return result == 0;
  }

  /// Overwrites [bytes] with zeros in memory.
  static void zeroize(Uint8List bytes) {
    bytes.fillRange(0, bytes.length, 0);
  }
}
