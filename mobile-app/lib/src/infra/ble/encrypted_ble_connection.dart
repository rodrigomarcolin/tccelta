import 'dart:async';
import 'dart:convert' show ascii;
import 'dart:typed_data';

import 'package:tccelta_mobile/src/core/crypto/psk_cipher.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';

/// A [BleConnection] decorator that transparently applies AES-256-GCM
/// encryption/decryption using a [PskCipher].
///
/// Wire format (hex-encoded, newline-terminated) matches the dongle's
/// `SecurePskBleConnectivity`:
///   OUTBOUND: `<12-byte IV hex><ciphertext hex><16-byte tag hex>\n`
///   INBOUND:  `<12-byte IV hex><ciphertext hex><16-byte tag hex>\n`
///
/// When a received frame fails authentication (wrong key / tampered bytes) it
/// is silently dropped — the [Elm327Client] above will time-out waiting for a
/// prompt, which is the same observable behaviour as the dongle discarding a
/// bad command.
class EncryptedBleConnection implements BleConnection {
  EncryptedBleConnection({
    required BleConnection inner,
    required PskCipher cipher,
  }) : _inner = inner,
       _cipher = cipher {
    _decryptedCtrl = StreamController<List<int>>.broadcast();
    _incomingSub = inner.incoming.listen(
      _onRawFrame,
      onDone: _decryptedCtrl.close,
    );
  }

  final BleConnection _inner;
  final PskCipher _cipher;

  late final StreamController<List<int>> _decryptedCtrl;
  late final StreamSubscription<List<int>> _incomingSub;

  // ── BleConnection interface ────────────────────────────────────────────────

  @override
  Stream<BleConnectionPhase> get phase => _inner.phase;

  @override
  BleConnectionPhase get currentPhase => _inner.currentPhase;

  @override
  bool get isReady => _inner.isReady;

  @override
  Stream<List<int>> get incoming => _decryptedCtrl.stream;

  /// Encrypts [bytes], hex-encodes the result and appends `\n`, then forwards
  /// to the inner connection's RX characteristic.
  @override
  Future<void> write(List<int> bytes) async {
    final encrypted = _cipher.encrypt(Uint8List.fromList(bytes));
    final hexFrame = '${_toHex(encrypted)}\n';
    await _inner.write(ascii.encode(hexFrame));
  }

  @override
  Future<void> disconnect() async {
    await _incomingSub.cancel();
    if (!_decryptedCtrl.isClosed) await _decryptedCtrl.close();
    await _inner.disconnect();
  }

  // ── Internal helpers ───────────────────────────────────────────────────────

  /// Accumulates incoming bytes until a newline, then decrypts the frame.
  final StringBuffer _lineBuffer = StringBuffer();

  void _onRawFrame(List<int> chunk) {
    _lineBuffer.write(ascii.decode(chunk, allowInvalid: true));
    // The dongle terminates each frame with \n; process all complete lines.
    var raw = _lineBuffer.toString();
    var nlIdx = raw.indexOf('\n');
    while (nlIdx >= 0) {
      final line = raw.substring(0, nlIdx).trim();
      raw = raw.substring(nlIdx + 1);
      nlIdx = raw.indexOf('\n');
      _processLine(line);
    }
    _lineBuffer
      ..clear()
      ..write(raw); // keep any partial line
  }

  void _processLine(String hexLine) {
    if (hexLine.isEmpty) return;
    final bytes = _fromHex(hexLine);
    if (bytes == null) return; // malformed hex
    final plain = _cipher.decrypt(bytes);
    if (plain == null) return; // auth failure — silently drop
    if (!_decryptedCtrl.isClosed) _decryptedCtrl.add(plain.toList());
  }

  // ── Hex helpers ────────────────────────────────────────────────────────────

  static String _toHex(Uint8List bytes) {
    final buf = StringBuffer();
    for (final b in bytes) {
      buf.write(b.toRadixString(16).padLeft(2, '0'));
    }
    return buf.toString();
  }

  /// Returns `null` if [hex] contains non-hex chars or has odd length.
  static Uint8List? _fromHex(String hex) {
    if (hex.length.isOdd) return null;
    try {
      final out = Uint8List(hex.length ~/ 2);
      for (var i = 0; i < out.length; i++) {
        out[i] = int.parse(hex.substring(i * 2, i * 2 + 2), radix: 16);
      }
      return out;
    } on FormatException {
      return null;
    }
  }
}
