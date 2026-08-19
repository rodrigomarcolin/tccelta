import 'dart:math';
import 'dart:typed_data';

import 'package:pointycastle/export.dart';

/// AES-256-GCM cipher over a pre-shared 32-byte key.
///
/// Wire format (raw bytes): `IV(12) || ciphertext(n) || TAG(16)`
/// This matches the dongle's `SecurePskBleConnectivity` — which additionally
/// hex-encodes the whole frame and appends `\n` before putting it on the wire.
/// The hex-encoding/decoding is handled by [EncryptedBleConnection]; this class
/// works purely in raw bytes.
class PskCipher {
  PskCipher({required Uint8List key}) : _key = key {
    if (key.length != 32) {
      throw ArgumentError(
        'AES-256 key must be exactly 32 bytes, '
        'got ${key.length}',
      );
    }
  }

  /// Creates a [PskCipher] from a 64-character hex string (case-insensitive).
  factory PskCipher.fromHex(String hex) {
    final clean = hex.trim();
    if (clean.length != 64) {
      throw ArgumentError(
        'PSK must be 64 hex chars (32 bytes), '
        'got ${clean.length} chars',
      );
    }
    final key = Uint8List(32);
    for (var i = 0; i < 32; i++) {
      key[i] = int.parse(clean.substring(i * 2, i * 2 + 2), radix: 16);
    }
    return PskCipher(key: key);
  }

  final Uint8List _key;

  static const int _ivLen = 12;
  static const int _tagLen = 16;

  /// Encrypts [plaintext] and returns `IV(12) || CT(n) || TAG(16)`.
  Uint8List encrypt(Uint8List plaintext) {
    final iv = _randomIv();
    final gcm = _buildCipher(forEncryption: true, iv: iv);

    // PointyCastle GCM output is CT + TAG concatenated.
    final ctAndTag = gcm.process(plaintext);

    final out = Uint8List(_ivLen + ctAndTag.length);
    out.setRange(0, _ivLen, iv);
    out.setRange(_ivLen, _ivLen + ctAndTag.length, ctAndTag);
    return out;
  }

  /// Decrypts a `IV(12) || CT(n) || TAG(16)` frame.
  ///
  /// Returns the plaintext on success, or `null` if the frame is too short or
  /// the authentication tag does not match (wrong key / tampered ciphertext).
  Uint8List? decrypt(Uint8List frame) {
    if (frame.length < _ivLen + _tagLen) return null;
    final iv = frame.sublist(0, _ivLen);
    final ctAndTag = frame.sublist(
      _ivLen,
    ); // CT + TAG (PointyCastle expects this)

    try {
      final gcm = _buildCipher(forEncryption: false, iv: iv);
      return gcm.process(ctAndTag);
    } on InvalidCipherTextException {
      return null; // authentication failure
    } on Object {
      return null;
    }
  }

  GCMBlockCipher _buildCipher({
    required bool forEncryption,
    required Uint8List iv,
  }) {
    final params = AEADParameters(
      KeyParameter(_key),
      _tagLen * 8, // tag size in bits
      iv,
      Uint8List(0), // no additional authenticated data
    );
    return GCMBlockCipher(AESEngine())..init(forEncryption, params);
  }

  static final Random _rng = Random.secure();

  static Uint8List _randomIv() {
    final iv = Uint8List(_ivLen);
    for (var i = 0; i < _ivLen; i++) {
      iv[i] = _rng.nextInt(256);
    }
    return iv;
  }
}
