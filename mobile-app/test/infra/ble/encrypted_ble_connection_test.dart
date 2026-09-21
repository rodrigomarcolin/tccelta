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

    test('performs handshake when inner reaches ready and emits ready',
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
    });

    test('outbound write encrypts with monotonic counter as AAD', () async {
      final cipher = PskCipher(key: testPsk);
      final encConn = EncryptedBleConnection.withCipher(
        inner: inner,
        cipher: cipher,
      );

      inner.emitPhase(BleConnectionPhase.ready);

      // Write command 1: "ATZ\r"
      await encConn.write(ascii.encode('ATZ\r'));
      expect(inner.written.length, equals(1));
      final frame1 = ascii.decode(inner.written[0]).trim();

      expect(frame1.length >= 64, isTrue);
      // Counter for 1st message is 0x00000001 (8 hex chars = "00000001")
      expect(frame1.substring(0, 8), equals('00000001'));

      final iv1 = _fromHex(frame1.substring(8, 32));
      final tag1 = _fromHex(frame1.substring(frame1.length - 32));
      final ct1 = _fromHex(frame1.substring(32, frame1.length - 32));
      final counterBytes1 = _fromHex(frame1.substring(0, 8));

      final decrypted1 = cipher.decryptSeparate(
        iv: iv1,
        ciphertext: ct1,
        tag: tag1,
        aad: counterBytes1,
      );
      expect(ascii.decode(decrypted1!), equals('ATZ\r'));

      // Write command 2: "010C\r" -> counter must be 0x00000002
      await encConn.write(ascii.encode('010C\r'));
      expect(inner.written.length, equals(2));
      final frame2 = ascii.decode(inner.written[1]).trim();
      expect(frame2.substring(0, 8), equals('00000002'));
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
      final counterBytes = Uint8List.fromList([0x00, 0x00, 0x00, 0x01]);
      final payload = Uint8List.fromList(ascii.encode('41 0C 1A F8\r\n>'));
      final enc = cipher.encryptSeparate(payload, aad: counterBytes);

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
      final counter5 = Uint8List.fromList([0x00, 0x00, 0x00, 0x05]);
      final enc5 = cipher.encryptSeparate(
        Uint8List.fromList(ascii.encode('RESP 5')),
        aad: counter5,
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
      final counter3 = Uint8List.fromList([0x00, 0x00, 0x00, 0x03]);
      final enc3 = cipher.encryptSeparate(
        Uint8List.fromList(ascii.encode('RESP 3')),
        aad: counter3,
      );
      final wire3 =
          '${_toHex(counter3)}${_toHex(enc3.iv)}'
          '${_toHex(enc3.ciphertext)}${_toHex(enc3.tag)}\n';
      inner.incomingCtrl.add(ascii.encode(wire3));

      await Future<void>.delayed(const Duration(milliseconds: 20));
      expect(received.length, equals(1));

      // 4. Newer message #6 -> Must be accepted
      final counter6 = Uint8List.fromList([0x00, 0x00, 0x00, 0x06]);
      final enc6 = cipher.encryptSeparate(
        Uint8List.fromList(ascii.encode('RESP 6')),
        aad: counter6,
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

      final counter1 = Uint8List.fromList([0x00, 0x00, 0x00, 0x01]);
      final enc1 = cipher.encryptSeparate(
        Uint8List.fromList(ascii.encode('GOOD')),
        aad: counter1,
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
