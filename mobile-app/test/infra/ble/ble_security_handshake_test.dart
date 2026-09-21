import 'dart:async';
import 'dart:convert' show ascii;
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/core/crypto/hkdf.dart';
import 'package:tccelta_mobile/src/infra/ble/ble_security_handshake.dart';

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

void main() {
  const testPskHex =
      '0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef';
  final testPsk = _fromHex(testPskHex);

  group('BleSecurityHandshake', () {
    late StreamController<List<int>> incomingDongleToApp;
    late List<String> sentAppToDongle;

    setUp(() {
      incomingDongleToApp = StreamController<List<int>>.broadcast();
      sentAppToDongle = [];
    });

    tearDown(() async {
      await incomingDongleToApp.close();
    });

    Future<void> mockWriteRaw(List<int> bytes) async {
      sentAppToDongle.add(ascii.decode(bytes));
    }

    test('successful handshake flow derives correct session key', () async {
      final handshake = BleSecurityHandshake(
        psk: testPsk,
        writeRaw: mockWriteRaw,
        timeout: const Duration(seconds: 2),
      );

      final serverNonce = Uint8List.fromList(
        List.generate(32, (i) => (i + 10) & 0xFF),
      );

      // Start handshake in background
      final handshakeFuture = handshake.perform(incomingDongleToApp.stream);

      // Wait for app to send HELLO
      await Future<void>.delayed(const Duration(milliseconds: 20));
      expect(sentAppToDongle.length, equals(1));
      final helloMsg = sentAppToDongle[0];
      expect(helloMsg.startsWith('HELLO '), isTrue);
      final clientNonceHex = helloMsg.substring('HELLO '.length).trim();
      final clientNonce = _fromHex(clientNonceHex);
      expect(clientNonce.length, equals(32));

      // Dongle sends CHALLENGE
      incomingDongleToApp.add(
        ascii.encode('CHALLENGE ${_toHex(serverNonce)}\n'),
      );

      // Wait for app to send PROOF
      await Future<void>.delayed(const Duration(milliseconds: 20));
      expect(sentAppToDongle.length, equals(2));
      final proofMsg = sentAppToDongle[1];
      expect(proofMsg.startsWith('PROOF '), isTrue);
      final clientProofHex = proofMsg.substring('PROOF '.length).trim();

      // Verify client proof locally
      final salt = Uint8List(64)
        ..setRange(0, 32, clientNonce)
        ..setRange(32, 64, serverNonce);
      final expectedProof = CryptoUtils.hmacSha256(testPsk, salt);
      expect(clientProofHex, equals(_toHex(expectedProof)));

      // Dongle derives session key and computes OK <confirmMac>
      final expectedSessionKey = CryptoUtils.hkdfSha256(
        ikm: testPsk,
        salt: salt,
        info: Uint8List.fromList(ascii.encode('session-key')),
      );
      final confirmMac = CryptoUtils.hmacSha256(
        expectedSessionKey,
        Uint8List.fromList(ascii.encode('confirm')),
      );

      incomingDongleToApp.add(ascii.encode('OK ${_toHex(confirmMac)}\n'));

      final derivedKey = await handshakeFuture;
      expect(_toHex(derivedKey), equals(_toHex(expectedSessionKey)));
    });

    test('throws when dongle responds with ERROR', () async {
      final handshake = BleSecurityHandshake(
        psk: testPsk,
        writeRaw: mockWriteRaw,
        timeout: const Duration(seconds: 2),
      );

      final serverNonce = Uint8List(32);
      final handshakeFuture = handshake.perform(incomingDongleToApp.stream);

      await Future<void>.delayed(const Duration(milliseconds: 20));
      incomingDongleToApp.add(
        ascii.encode('CHALLENGE ${_toHex(serverNonce)}\n'),
      );

      await Future<void>.delayed(const Duration(milliseconds: 20));
      incomingDongleToApp.add(ascii.encode('ERROR\n'));

      expect(
        () => handshakeFuture,
        throwsA(
          isA<BleHandshakeException>().having(
            (e) => e.message,
            'message',
            contains('ERROR received'),
          ),
        ),
      );
    });

    test('throws when dongle sends invalid confirm MAC', () async {
      final handshake = BleSecurityHandshake(
        psk: testPsk,
        writeRaw: mockWriteRaw,
        timeout: const Duration(seconds: 2),
      );

      final serverNonce = Uint8List(32);
      final handshakeFuture = handshake.perform(incomingDongleToApp.stream);

      await Future<void>.delayed(const Duration(milliseconds: 20));
      incomingDongleToApp.add(
        ascii.encode('CHALLENGE ${_toHex(serverNonce)}\n'),
      );

      await Future<void>.delayed(const Duration(milliseconds: 20));
      // Send bogus 32-byte MAC
      final badMac = Uint8List(32);
      incomingDongleToApp.add(ascii.encode('OK ${_toHex(badMac)}\n'));

      expect(
        () => handshakeFuture,
        throwsA(
          isA<BleHandshakeException>().having(
            (e) => e.message,
            'message',
            contains('confirmation MAC mismatch'),
          ),
        ),
      );
    });

    test('throws on timeout if dongle does not respond', () async {
      final handshake = BleSecurityHandshake(
        psk: testPsk,
        writeRaw: mockWriteRaw,
        timeout: const Duration(milliseconds: 50),
      );

      final handshakeFuture = handshake.perform(incomingDongleToApp.stream);

      expect(
        () => handshakeFuture,
        throwsA(
          isA<BleHandshakeException>().having(
            (e) => e.message,
            'message',
            contains('timeout'),
          ),
        ),
      );
    });
  });
}
