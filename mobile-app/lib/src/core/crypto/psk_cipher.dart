import 'dart:math';
import 'dart:typed_data';

import 'package:pointycastle/export.dart';

/// AES-256-GCM cipher over a pre-shared 32-byte key.
///
/// Wire format (raw bytes): `IV(12) || ciphertext(n) || TAG(16)`
/// This matches the dongle's `SecurePskBleConnectivity` — which additionally
/// hex-encodes the whole frame and appends `\n` before putting it on the wire.
/// The hex-encoding/decoding is handled by `EncryptedBleConnection`; this
/// class works purely in raw bytes.
class PskCipher {
  /// Creates a cipher bound to a raw 32-byte AES-256 [key].
  PskCipher({required Uint8List key}) : _key = Uint8List.fromList(key) {
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

  /// `true` iff [hex] is exactly 64 hex chars (case-insensitive) — the same
  /// rule [PskCipher.fromHex] enforces, but as a non-throwing predicate.
  static bool isValidHex(String hex) =>
      RegExp(r'^[0-9a-fA-F]{64}$').hasMatch(hex.trim());

  final Uint8List _key;

  /// Returns a defensive copy for protocols that use the PSK during a
  /// handshake. The cipher keeps ownership of its internal key.
  Uint8List get keyCopy => Uint8List.fromList(_key);

  static const int _ivLen = 12;
  static const int _tagLen = 16;

  /// Encrypts [plaintext] and returns `IV(12) || CT(n) || TAG(16)`.
  Uint8List encrypt(Uint8List plaintext) {
    final parts = encryptSeparate(plaintext);
    return Uint8List(_ivLen + parts.ciphertext.length + _tagLen)
      ..setRange(0, _ivLen, parts.iv)
      ..setRange(_ivLen, _ivLen + parts.ciphertext.length, parts.ciphertext)
      ..setRange(
        _ivLen + parts.ciphertext.length,
        _ivLen + parts.ciphertext.length + parts.tag.length,
        parts.tag,
      );
  }

  /// Decrypts a `IV(12) || CT(n) || TAG(16)` frame.
  ///
  /// Returns the plaintext on success, or `null` if the frame is too short or
  /// the authentication tag does not match (wrong key / tampered ciphertext).
  Uint8List? decrypt(Uint8List frame) {
    if (frame.length < _ivLen + _tagLen) return null;
    return decryptSeparate(
      iv: frame.sublist(0, _ivLen),
      ciphertext: frame.sublist(_ivLen, frame.length - _tagLen),
      tag: frame.sublist(frame.length - _tagLen),
    );
  }

  /// Encrypts with optional authenticated additional data, keeping GCM parts
  /// separate for the replay-counter wire format.
  EncryptedParts encryptSeparate(
    Uint8List plaintext, {
    Uint8List? aad,
  }) {
    final iv = _randomIv();
    final gcm = _buildCipher(
      forEncryption: true,
      iv: iv,
      aad: aad ?? Uint8List(0),
    );
    final ctAndTag = gcm.process(plaintext);
    return EncryptedParts(
      iv: iv,
      ciphertext: Uint8List.sublistView(ctAndTag, 0, ctAndTag.length - _tagLen),
      tag: Uint8List.sublistView(ctAndTag, ctAndTag.length - _tagLen),
    );
  }

  /// Decrypts separate GCM parts with optional authenticated additional data.
  Uint8List? decryptSeparate({
    required Uint8List iv,
    required Uint8List ciphertext,
    required Uint8List tag,
    Uint8List? aad,
  }) {
    if (iv.length != _ivLen || tag.length != _tagLen) return null;
    try {
      final gcm = _buildCipher(
        forEncryption: false,
        iv: iv,
        aad: aad ?? Uint8List(0),
      );
      return gcm.process(Uint8List.fromList([...ciphertext, ...tag]));
    } on InvalidCipherTextException {
      return null;
    } on Object {
      return null;
    }
  }

  GCMBlockCipher _buildCipher({
    required bool forEncryption,
    required Uint8List iv,
    required Uint8List aad,
  }) {
    final params = AEADParameters(
      KeyParameter(_key),
      _tagLen * 8, // tag size in bits
      iv,
      aad,
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

/// The three independently encoded AES-GCM parts used by replay framing.
class EncryptedParts {
  /// Creates a set of separately encoded AES-GCM parts.
  const EncryptedParts({
    required this.iv,
    required this.ciphertext,
    required this.tag,
  });

  /// The 12-byte initialization vector (nonce).
  final Uint8List iv;

  /// The encrypted payload, excluding the authentication tag.
  final Uint8List ciphertext;

  /// The 16-byte GCM authentication tag.
  final Uint8List tag;
}
