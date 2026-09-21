import 'dart:async';
import 'dart:convert' show ascii;
import 'dart:typed_data';

import 'package:tccelta_mobile/src/core/crypto/psk_cipher.dart';
import 'package:tccelta_mobile/src/core/errors/ble_failure.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';
import 'package:tccelta_mobile/src/domain/ble/security_mode.dart';
import 'package:tccelta_mobile/src/infra/ble/ble_security_handshake.dart';

/// Secure BLE decorator for the three firmware security variants.
class EncryptedBleConnection implements BleConnection {
  /// Wraps [inner] with encryption for the selected security [mode].
  factory EncryptedBleConnection({
    required BleConnection inner,
    Uint8List? psk,
    PskCipher? cipher,
    SecurityMode mode = SecurityMode.handshake,
    Duration handshakeTimeout = const Duration(seconds: 10),
  }) {
    if (mode == SecurityMode.staticPsk && cipher == null && psk == null) {
      throw ArgumentError('Static PSK mode requires a PSK or cipher');
    }
    return EncryptedBleConnection._(inner, psk, cipher, mode, handshakeTimeout)
      .._start();
  }

  /// Test/compatibility constructor for an already-established cipher. It
  /// uses replay framing so counter behavior can be exercised directly.
  factory EncryptedBleConnection.withCipher({
    required BleConnection inner,
    required PskCipher cipher,
  }) => EncryptedBleConnection._(
    inner,
    null,
    cipher,
    SecurityMode.handshakeReplay,
    const Duration(seconds: 10),
  ).._startEstablished();

  EncryptedBleConnection._(
    this._inner,
    this._psk,
    this._cipher,
    this._mode,
    this._handshakeTimeout,
  );

  void _start() {
    _phaseSub = _inner.phase.listen(_onInnerPhase);
    if (_mode == SecurityMode.staticPsk) _listenEncryptedFrames();
    if (_inner.currentPhase != BleConnectionPhase.ready) {
      _emit(_inner.currentPhase);
    } else if (_mode == SecurityMode.staticPsk) {
      _established = true;
      _emit(BleConnectionPhase.ready);
    } else {
      unawaited(_startSecureSession());
    }
  }

  void _startEstablished() {
    _established = true;
    _phaseSub = _inner.phase.listen(_onInnerPhase);
    _listenEncryptedFrames();
    _emit(_inner.currentPhase);
  }

  final BleConnection _inner;
  final Uint8List? _psk;
  PskCipher? _cipher;
  final SecurityMode _mode;
  final Duration _handshakeTimeout;
  final StreamController<BleConnectionPhase> _phaseCtrl =
      StreamController<BleConnectionPhase>.broadcast();
  final StreamController<List<int>> _decryptedCtrl =
      StreamController<List<int>>.broadcast();
  late final StreamSubscription<BleConnectionPhase> _phaseSub;
  StreamSubscription<List<int>>? _incomingSub;
  final StringBuffer _lineBuffer = StringBuffer();
  BleConnectionPhase _currentPhase = BleConnectionPhase.idle;
  bool _established = false;
  bool _starting = false;
  bool _disposed = false;
  int _txCounter = 0;
  int _rxCounter = 0;
  bool _rxCounterInitialized = false;

  @override
  Stream<BleConnectionPhase> get phase => _phaseCtrl.stream;

  @override
  BleConnectionPhase get currentPhase => _currentPhase;

  @override
  bool get isReady => _established && _currentPhase == BleConnectionPhase.ready;

  @override
  Stream<List<int>> get incoming => _decryptedCtrl.stream;

  void _onInnerPhase(BleConnectionPhase phase) {
    if (_disposed) return;
    if (phase != BleConnectionPhase.ready) {
      _established = false;
      final incoming = _incomingSub;
      _incomingSub = null;
      if (incoming != null) unawaited(incoming.cancel());
      _emit(phase);
      return;
    }
    if (_established && _cipher != null) {
      _emit(BleConnectionPhase.ready);
      return;
    }
    if (_mode == SecurityMode.staticPsk) {
      _established = true;
      _listenEncryptedFrames();
      _emit(BleConnectionPhase.ready);
    } else {
      unawaited(_startSecureSession());
    }
  }

  Future<void> _startSecureSession() async {
    if (_starting || _disposed || _established) return;
    _starting = true;
    _emit(BleConnectionPhase.connecting);
    try {
      final psk = _psk;
      if (psk == null || psk.length != 32) {
        throw const BleConnectionFailure('PSK inválida para handshake');
      }
      final sessionKey = await BleSecurityHandshake(
        psk: psk,
        writeRaw: _inner.write,
        timeout: _handshakeTimeout,
      ).perform(_inner.incoming);
      _cipher = PskCipher(key: sessionKey);
      sessionKey.fillRange(0, sessionKey.length, 0);
      _txCounter = 0;
      _rxCounter = 0;
      _rxCounterInitialized = false;
      _established = true;
      _listenEncryptedFrames();
      _emit(BleConnectionPhase.ready);
    } on Object {
      _established = false;
      _emit(BleConnectionPhase.failed);
      if (!_disposed) unawaited(_inner.disconnect());
    } finally {
      _starting = false;
    }
  }

  @override
  Future<void> write(List<int> bytes) async {
    if (!isReady || _cipher == null) {
      throw const BleConnectionFailure('Canal seguro ainda não está pronto');
    }
    final payload = Uint8List.fromList(bytes);
    final frame = _mode == SecurityMode.handshakeReplay
        ? _encryptReplay(payload)
        : _cipher!.encrypt(payload);
    await _inner.write(ascii.encode('${_toHex(frame)}\n'));
  }

  Uint8List _encryptReplay(Uint8List payload) {
    final counter = _counterBytes(_txCounter++);
    final aad = Uint8List(9)
      ..setRange(0, 8, counter)
      ..[8] = 0;
    final parts = _cipher!.encryptSeparate(payload, aad: aad);
    return Uint8List.fromList([
      ...counter,
      ...parts.iv,
      ...parts.ciphertext,
      ...parts.tag,
    ]);
  }

  void _onRawFrame(List<int> chunk) {
    if (_disposed) return;
    _lineBuffer.write(ascii.decode(chunk, allowInvalid: true));
    var raw = _lineBuffer.toString();
    var nl = raw.indexOf('\n');
    while (nl >= 0) {
      final line = raw.substring(0, nl).trim();
      raw = raw.substring(nl + 1);
      if (line.isNotEmpty) _processLine(line);
      nl = raw.indexOf('\n');
    }
    _lineBuffer
      ..clear()
      ..write(raw);
  }

  void _listenEncryptedFrames() {
    _incomingSub ??= _inner.incoming.listen(_onRawFrame);
  }

  void _processLine(String line) {
    if (!_established || _cipher == null) return;
    final frame = _fromHex(line);
    if (frame == null) return;
    final plain = _mode == SecurityMode.handshakeReplay
        ? _decryptReplay(frame)
        : _cipher!.decrypt(frame);
    if (plain != null && !_decryptedCtrl.isClosed) {
      _decryptedCtrl.add(plain.toList());
    }
  }

  Uint8List? _decryptReplay(Uint8List frame) {
    if (frame.length < 8 + 12 + 16) return null;
    final counterBytes = frame.sublist(0, 8);
    final counter = _counterValue(counterBytes);
    if (_rxCounterInitialized && counter <= _rxCounter) return null;
    final aad = Uint8List(9)
      ..setRange(0, 8, counterBytes)
      ..[8] = 1;
    final plain = _cipher!.decryptSeparate(
      iv: frame.sublist(8, 20),
      ciphertext: frame.sublist(20, frame.length - 16),
      tag: frame.sublist(frame.length - 16),
      aad: aad,
    );
    if (plain == null) return null;
    _rxCounter = counter;
    _rxCounterInitialized = true;
    return plain;
  }

  void _emit(BleConnectionPhase phase) {
    _currentPhase = phase;
    if (!_phaseCtrl.isClosed) _phaseCtrl.add(phase);
  }

  @override
  Future<void> disconnect() async {
    _disposed = true;
    await _phaseSub.cancel();
    await _incomingSub?.cancel();
    await _decryptedCtrl.close();
    await _phaseCtrl.close();
    await _inner.disconnect();
  }

  static Uint8List _counterBytes(int value) {
    final bytes = Uint8List(8);
    var remaining = value;
    for (var i = 7; i >= 0; i--) {
      bytes[i] = remaining & 0xff;
      remaining >>= 8;
    }
    return bytes;
  }

  static int _counterValue(List<int> bytes) {
    var value = 0;
    for (final byte in bytes) {
      value = (value << 8) | byte;
    }
    return value;
  }

  static String _toHex(List<int> bytes) => bytes
      .map((byte) => byte.toRadixString(16).padLeft(2, '0'))
      .join();

  static Uint8List? _fromHex(String hex) {
    if (hex.isEmpty || hex.length.isOdd) return null;
    try {
      return Uint8List.fromList([
        for (var i = 0; i < hex.length; i += 2)
          int.parse(hex.substring(i, i + 2), radix: 16),
      ]);
    } on FormatException {
      return null;
    }
  }
}
