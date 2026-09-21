import 'dart:async';
import 'dart:convert' show ascii;
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/core/crypto/hkdf.dart';
import 'package:tccelta_mobile/src/core/crypto/psk_cipher.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';
import 'package:tccelta_mobile/src/infra/ble/encrypted_ble_connection.dart';

Uint8List _fromHex(String hex) {
  final clean = hex.trim();
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

/// 8-byte big-endian replay counter, matching the firmware's
/// `SecureHandshakeReplayBleConnectivity` wire format.
Uint8List _replayCounterBytes(int value) {
  final bytes = Uint8List(8);
  var remaining = value;
  for (var i = 7; i >= 0; i--) {
    bytes[i] = remaining & 0xff;
    remaining >>= 8;
  }
  return bytes;
}

const _dirAppToDongle = 0x00;
const _dirDongleToApp = 0x01;

/// GCM AAD used by the replay-protected mode: counter(8) ‖ direction(1).
Uint8List _replayAad(Uint8List counterBytes, int direction) =>
    Uint8List.fromList([...counterBytes, direction]);

class FakeBleConnection implements BleConnection {
  final StreamController<BleConnectionPhase> phaseCtrl =
      StreamController<BleConnectionPhase>.broadcast();
  final StreamController<List<int>> incomingCtrl =
      StreamController<List<int>>.broadcast();

  BleConnectionPhase _currentPhase = BleConnectionPhase.idle;
  final List<List<int>> written = [];
  bool disconnected = false;

  @override
  BleConnectionPhase get currentPhase => _currentPhase;

  @override
  Stream<List<int>> get incoming => incomingCtrl.stream;

  @override
  bool get isReady => _currentPhase == BleConnectionPhase.ready;

  @override
  Stream<BleConnectionPhase> get phase => phaseCtrl.stream;

  void emitPhase(BleConnectionPhase p) {
    _currentPhase = p;
    if (!phaseCtrl.isClosed) phaseCtrl.add(p);
  }

  @override
  Future<void> write(List<int> bytes) async {
    written.add(bytes);
  }

  @override
  Future<void> disconnect() async {
    disconnected = true;
    emitPhase(BleConnectionPhase.disconnected);
    await phaseCtrl.close();
    await incomingCtrl.close();
  }
}

void main() {
  const testPskHex =
      '0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef';
  final testPsk = _fromHex(testPskHex);

  group('EncryptedBleConnection', () {
    late FakeBleConnection inner;

    setUp(() {
      inner = FakeBleConnection();
    });

    test(
      'performs handshake when inner reaches ready and emits ready',
      () async {
        final encConn = EncryptedBleConnection(
          inner: inner,
          psk: testPsk,
          handshakeTimeout: const Duration(seconds: 2),
        );

        final phases = <BleConnectionPhase>[];
        final phaseSub = encConn.phase.listen(phases.add);

        // Inner reaches ready -> triggers handshake
        inner.emitPhase(BleConnectionPhase.ready);

        await Future<void>.delayed(const Duration(milliseconds: 20));
        expect(inner.written.length, equals(1));
        final hello = ascii.decode(inner.written[0]);
        expect(hello.startsWith('HELLO '), isTrue);
        final clientNonce = _fromHex(hello.substring('HELLO '.length).trim());

        // Dongle sends challenge
        final serverNonce = Uint8List(32);
        inner.incomingCtrl.add(
          ascii.encode('CHALLENGE ${_toHex(serverNonce)}\n'),
        );

        await Future<void>.delayed(const Duration(milliseconds: 20));
        expect(inner.written.length, equals(2));
        final proofMsg = ascii.decode(inner.written[1]);
        expect(proofMsg.startsWith('PROOF '), isTrue);

        // Dongle sends OK with valid confirmation MAC
        final salt = Uint8List(64)
          ..setRange(0, 32, clientNonce)
          ..setRange(32, 64, serverNonce);
        final sessionKey = CryptoUtils.hkdfSha256(
          ikm: testPsk,
          salt: salt,
          info: Uint8List.fromList(ascii.encode('session-key')),
        );
        final confirmMac = CryptoUtils.hmacSha256(
          sessionKey,
          Uint8List.fromList(ascii.encode('confirm')),
        );
        inner.incomingCtrl.add(ascii.encode('OK ${_toHex(confirmMac)}\n'));

        await Future<void>.delayed(const Duration(milliseconds: 30));
        expect(encConn.isReady, isTrue);
        expect(encConn.currentPhase, equals(BleConnectionPhase.ready));

        await phaseSub.cancel();
        await encConn.disconnect();
      },
    );

    test('outbound write encrypts with monotonic counter as AAD', () async {
      final cipher = PskCipher(key: testPsk);
      final encConn = EncryptedBleConnection.withCipher(
        inner: inner,
        cipher: cipher,
      );

      inner.emitPhase(BleConnectionPhase.ready);
      // Phase updates are delivered asynchronously over a broadcast stream.
      await Future<void>.delayed(const Duration(milliseconds: 20));

      // Write command 1: "ATZ\r"
      await encConn.write(ascii.encode('ATZ\r'));
      expect(inner.written.length, equals(1));
      final frame1 = ascii.decode(inner.written[0]).trim();

      expect(frame1.length >= 72, isTrue);
      // Counter starts at 0 (post-incremented, matching the firmware's
      // `uint64_t counter = _txCounter++;`), 16 hex chars = 8-byte
      // big-endian.
      expect(frame1.substring(0, 16), equals('0000000000000000'));

      final counterBytes1 = _fromHex(frame1.substring(0, 16));
      final iv1 = _fromHex(frame1.substring(16, 40));
      final tag1 = _fromHex(frame1.substring(frame1.length - 32));
      final ct1 = _fromHex(frame1.substring(40, frame1.length - 32));

      final decrypted1 = cipher.decryptSeparate(
        iv: iv1,
        ciphertext: ct1,
        tag: tag1,
        aad: _replayAad(counterBytes1, _dirAppToDongle),
      );
      expect(ascii.decode(decrypted1!), equals('ATZ\r'));

      // Write command 2: "010C\r" -> counter must be 1
      await encConn.write(ascii.encode('010C\r'));
      expect(inner.written.length, equals(2));
      final frame2 = ascii.decode(inner.written[1]).trim();
      expect(frame2.substring(0, 16), equals('0000000000000001'));
    });

    test('inbound decrypted stream receives valid frame', () async {
      final cipher = PskCipher(key: testPsk);
      final encConn = EncryptedBleConnection.withCipher(
        inner: inner,
        cipher: cipher,
      );

      final receivedPlaintext = <String>[];
      final sub = encConn.incoming.listen((bytes) {
        receivedPlaintext.add(ascii.decode(bytes));
      });

      // Dongle sends message with counter = 1
      final counterBytes = _replayCounterBytes(1);
      final payload = Uint8List.fromList(ascii.encode('41 0C 1A F8\r\n>'));
      final enc = cipher.encryptSeparate(
        payload,
        aad: _replayAad(counterBytes, _dirDongleToApp),
      );

      final wireFrame =
          '${_toHex(counterBytes)}${_toHex(enc.iv)}'
          '${_toHex(enc.ciphertext)}${_toHex(enc.tag)}\n';

      inner.incomingCtrl.add(ascii.encode(wireFrame));

      await Future<void>.delayed(const Duration(milliseconds: 20));
      expect(receivedPlaintext.length, equals(1));
      expect(receivedPlaintext[0], equals('41 0C 1A F8\r\n>'));

      await sub.cancel();
    });

    test('inbound rejects replay attack (counter <= recvCounter)', () async {
      final cipher = PskCipher(key: testPsk);
      final encConn = EncryptedBleConnection.withCipher(
        inner: inner,
        cipher: cipher,
      );

      final received = <String>[];
      final sub = encConn.incoming.listen((bytes) {
        received.add(ascii.decode(bytes));
      });

      // 1. Dongle sends message #5
      final counter5 = _replayCounterBytes(5);
      final enc5 = cipher.encryptSeparate(
        Uint8List.fromList(ascii.encode('RESP 5')),
        aad: _replayAad(counter5, _dirDongleToApp),
      );
      final wire5 =
          '${_toHex(counter5)}${_toHex(enc5.iv)}'
          '${_toHex(enc5.ciphertext)}${_toHex(enc5.tag)}\n';
      inner.incomingCtrl.add(ascii.encode(wire5));

      await Future<void>.delayed(const Duration(milliseconds: 20));
      expect(received.length, equals(1));
      expect(received[0], equals('RESP 5'));

      // 2. Replay message #5 -> Must be rejected
      inner.incomingCtrl.add(ascii.encode(wire5));
      await Future<void>.delayed(const Duration(milliseconds: 20));
      expect(received.length, equals(1)); // No new message

      // 3. Replay older message #3 -> Must be rejected
      final counter3 = _replayCounterBytes(3);
      final enc3 = cipher.encryptSeparate(
        Uint8List.fromList(ascii.encode('RESP 3')),
        aad: _replayAad(counter3, _dirDongleToApp),
      );
      final wire3 =
          '${_toHex(counter3)}${_toHex(enc3.iv)}'
          '${_toHex(enc3.ciphertext)}${_toHex(enc3.tag)}\n';
      inner.incomingCtrl.add(ascii.encode(wire3));

      await Future<void>.delayed(const Duration(milliseconds: 20));
      expect(received.length, equals(1));

      // 4. Newer message #6 -> Must be accepted
      final counter6 = _replayCounterBytes(6);
      final enc6 = cipher.encryptSeparate(
        Uint8List.fromList(ascii.encode('RESP 6')),
        aad: _replayAad(counter6, _dirDongleToApp),
      );
      final wire6 =
          '${_toHex(counter6)}${_toHex(enc6.iv)}'
          '${_toHex(enc6.ciphertext)}${_toHex(enc6.tag)}\n';
      inner.incomingCtrl.add(ascii.encode(wire6));

      await Future<void>.delayed(const Duration(milliseconds: 20));
      expect(received.length, equals(2));
      expect(received[1], equals('RESP 6'));

      await sub.cancel();
    });

    test('inbound rejects tampered ciphertext or bad tag', () async {
      final cipher = PskCipher(key: testPsk);
      final encConn = EncryptedBleConnection.withCipher(
        inner: inner,
        cipher: cipher,
      );

      final received = <String>[];
      final sub = encConn.incoming.listen((bytes) {
        received.add(ascii.decode(bytes));
      });

      final counter1 = _replayCounterBytes(1);
      final enc1 = cipher.encryptSeparate(
        Uint8List.fromList(ascii.encode('GOOD')),
        aad: _replayAad(counter1, _dirDongleToApp),
      );

      // Tamper ciphertext
      final tamperedCt = Uint8List.fromList(enc1.ciphertext);
      tamperedCt[0] ^= 0xFF;

      final wireTampered =
          '${_toHex(counter1)}${_toHex(enc1.iv)}'
          '${_toHex(tamperedCt)}${_toHex(enc1.tag)}\n';
      inner.incomingCtrl.add(ascii.encode(wireTampered));

      await Future<void>.delayed(const Duration(milliseconds: 20));
      expect(received.isEmpty, isTrue); // Silently dropped

      await sub.cancel();
    });
  });
}
