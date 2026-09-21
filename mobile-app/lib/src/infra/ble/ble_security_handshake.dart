import 'dart:async';
import 'dart:convert' show ascii, utf8;
import 'dart:math';
import 'dart:typed_data';

import 'package:tccelta_mobile/src/core/crypto/hkdf.dart';

/// Exceptions thrown during the BLE security handshake.
class BleHandshakeException implements Exception {
  /// Creates a handshake exception with descriptive [message].
  BleHandshakeException(this.message);

  /// Human-readable message detailing the failure reason.
  final String message;

  @override
  String toString() => 'BleHandshakeException: $message';
}

/// Implements Phase 1 — Application-layer Mutual-Authentication Handshake:
///
/// 1. Initiate (App -> Dongle):
///    Generate fresh 32-byte CSPRNG random `clientNonce`.
///    Send `HELLO <clientNonce:64hex>\n`
/// 2. Receive Challenge (Dongle -> App):
///    Receive `CHALLENGE <serverNonce:64hex>\n`
/// 3. Send Proof (App -> Dongle):
///    Compute `salt = clientNonce || serverNonce` (64 bytes).
///    Compute `clientProof = HMAC-SHA256(key=PSK, data=salt)`.
///    Send `PROOF <clientProof:64hex>\n`
/// 4. Verify Dongle Confirmation (Dongle -> App):
///    Receive `OK <confirmMac:64hex>\n` (or `ERROR\n` on failure).
///    Derive session key:
///      `sessionKey = HKDF-SHA256(ikm=PSK, salt=salt, info="session-key")`
///    Compute
///      `expectedConfirmMac = HMAC-SHA256(key=sessionKey, data="confirm")`
///    Perform constant-time comparison of `confirmMac` & `expectedConfirmMac`.
///    If match: returns derived 32-byte `sessionKey`.
///    If mismatch or ERROR: throws [BleHandshakeException], wiping memory.
class BleSecurityHandshake {
  /// Creates a handshake runner with [psk], write callback [writeRaw],
  /// and [timeout].
  factory BleSecurityHandshake({
    required Uint8List psk,
    required Future<void> Function(List<int> bytes) writeRaw,
    Duration timeout = const Duration(seconds: 10),
  }) {
    if (psk.length != 32) {
      throw ArgumentError('PSK must be exactly 32 bytes');
    }
    return BleSecurityHandshake._(Uint8List.fromList(psk), writeRaw, timeout);
  }

  BleSecurityHandshake._(this._psk, this._writeRaw, this._timeout);

  final Uint8List _psk;
  final Future<void> Function(List<int> bytes) _writeRaw;
  final Duration _timeout;

  static const String _hkdfInfo = 'session-key';
  static const String _confirmLabel = 'confirm';
  static const int _nonceLen = 32;

  final StringBuffer _rxBuffer = StringBuffer();
  StreamController<String>? _linesController;

  static final Random _rng = Random.secure();

  /// Executes the handshake against an incoming byte stream.
  /// Returns the derived 32-byte `sessionKey` on success.
  Future<Uint8List> perform(Stream<List<int>> incoming) async {
    final clientNonce = _generateNonce();
    Uint8List? serverNonce;
    Uint8List? salt;
    Uint8List? sessionKey;

    _linesController = StreamController<String>.broadcast();
    final sub = incoming.listen(
      (chunk) => _onIncomingChunk(chunk, _linesController!),
      onError: (Object e) {
        if (!_linesController!.isClosed) {
          _linesController!.addError(e);
        }
      },
    );

    try {
      final linesStream = _linesController!.stream;

      // 1. Send HELLO
      final helloMsg = 'HELLO ${_toHex(clientNonce)}\n';
      await _writeRaw(ascii.encode(helloMsg));

      // 2. Receive CHALLENGE
      final challengeLine = await _receiveLine(linesStream, _timeout);
      if (!challengeLine.startsWith('CHALLENGE ')) {
        throw BleHandshakeException(
          'Expected CHALLENGE frame, got: $challengeLine',
        );
      }
      final serverNonceHex =
          challengeLine.substring('CHALLENGE '.length).trim();
      serverNonce = _parseHex(serverNonceHex, expectedLen: _nonceLen);
      if (serverNonce == null) {
        throw BleHandshakeException(
          'Invalid server nonce in CHALLENGE: $serverNonceHex',
        );
      }

      // 3. Compute salt and proof
      salt = Uint8List(64)
        ..setRange(0, 32, clientNonce)
        ..setRange(32, 64, serverNonce);

      final clientProof = CryptoUtils.hmacSha256(_psk, salt);
      final proofMsg = 'PROOF ${_toHex(clientProof)}\n';
      await _writeRaw(ascii.encode(proofMsg));
      CryptoUtils.zeroize(clientProof);

      // 4. Receive OK / ERROR
      final okLine = await _receiveLine(linesStream, _timeout);
      if (okLine.startsWith('ERROR')) {
        throw BleHandshakeException('Dongle rejected proof (ERROR received)');
      }
      if (!okLine.startsWith('OK ')) {
        throw BleHandshakeException('Expected OK frame, got: $okLine');
      }
      final confirmMacHex = okLine.substring('OK '.length).trim();
      final confirmMac = _parseHex(confirmMacHex, expectedLen: 32);
      if (confirmMac == null) {
        throw BleHandshakeException(
          'Invalid confirm MAC in OK response: $confirmMacHex',
        );
      }

      // 5. Derive session key and verify mutual confirmation
      sessionKey = CryptoUtils.hkdfSha256(
        ikm: _psk,
        salt: salt,
        info: Uint8List.fromList(ascii.encode(_hkdfInfo)),
      );

      final expectedConfirmMac = CryptoUtils.hmacSha256(
        sessionKey,
        Uint8List.fromList(ascii.encode(_confirmLabel)),
      );

      final isMatch = CryptoUtils.constantTimeEquals(
        confirmMac,
        expectedConfirmMac,
      );

      CryptoUtils.zeroize(expectedConfirmMac);
      CryptoUtils.zeroize(confirmMac);

      if (!isMatch) {
        throw BleHandshakeException(
          'Mutual authentication confirmation MAC mismatch',
        );
      }

      return sessionKey;
    } catch (e) {
      if (sessionKey != null) {
        CryptoUtils.zeroize(sessionKey);
      }
      rethrow;
    } finally {
      await sub.cancel();
      await _linesController?.close();
      _rxBuffer.clear();

      CryptoUtils.zeroize(clientNonce);
      if (serverNonce != null) CryptoUtils.zeroize(serverNonce);
      if (salt != null) CryptoUtils.zeroize(salt);
    }
  }

  void _onIncomingChunk(List<int> chunk, StreamController<String> ctrl) {
    _rxBuffer.write(utf8.decode(chunk, allowMalformed: true));
    var content = _rxBuffer.toString();
    var nlIdx = content.indexOf('\n');
    while (nlIdx >= 0) {
      final line = content.substring(0, nlIdx).replaceAll('\r', '').trim();
      content = content.substring(nlIdx + 1);
      nlIdx = content.indexOf('\n');
      if (line.isNotEmpty && !ctrl.isClosed) {
        ctrl.add(line);
      }
    }
    _rxBuffer
      ..clear()
      ..write(content);
  }

  Future<String> _receiveLine(Stream<String> stream, Duration timeout) async {
    return stream.first.timeout(
      timeout,
      onTimeout: () =>
          throw BleHandshakeException('Handshake timeout waiting for response'),
    );
  }

  static Uint8List _generateNonce() {
    final bytes = Uint8List(_nonceLen);
    for (var i = 0; i < _nonceLen; i++) {
      bytes[i] = _rng.nextInt(256);
    }
    return bytes;
  }

  static String _toHex(Uint8List bytes) {
    final buf = StringBuffer();
    for (final b in bytes) {
      buf.write(b.toRadixString(16).padLeft(2, '0'));
    }
    return buf.toString();
  }

  static Uint8List? _parseHex(String hex, {required int expectedLen}) {
    final clean = hex.trim();
    if (clean.length != expectedLen * 2) return null;
    try {
      final out = Uint8List(expectedLen);
      for (var i = 0; i < expectedLen; i++) {
        out[i] = int.parse(clean.substring(i * 2, i * 2 + 2), radix: 16);
      }
      return out;
    } on FormatException {
      return null;
    }
  }
}
