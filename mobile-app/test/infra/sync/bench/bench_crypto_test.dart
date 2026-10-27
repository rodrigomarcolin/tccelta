import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/infra/sync/bench/bench_crypto.dart';
import 'package:tccelta_mobile/src/infra/sync/bench/bench_keys.dart';

import '../../../support/sync_known_vectors.dart';

void main() {
  test('chaves padrão são as chaves fixas de bancada', () {
    final keys = BenchKeys.defaults();
    expect(keys.kAuth, SyncVectors.kAuth);
    expect(keys.kRec, SyncVectors.kRec);
  });

  group('tags de handshake (16 B)', () {
    Future<void> check(int type, List<int> payload, List<int> tag) async {
      expect(
        handshakeTag(
          kAuth: SyncVectors.kAuth,
          type: type,
          devId: SyncVectors.devId,
          payload: Uint8List.fromList(payload),
        ),
        tag,
      );
    }

    test(
      'desafio',
      () => check(
        0x01,
        SyncVectors.challengePayload,
        SyncVectors.challengeTag,
      ),
    );
    test(
      'desafio com diagnóstico',
      () => check(
        0x01,
        SyncVectors.challengeDiagPayload,
        SyncVectors.challengeDiagTag,
      ),
    );
    test(
      'resposta',
      () => check(
        0x02,
        SyncVectors.responsePayload,
        SyncVectors.responseTag,
      ),
    );
    test('ACK', () => check(0x04, SyncVectors.ackPayload, SyncVectors.ackTag));
  });

  group('tags de bloco (32 B)', () {
    test('BOOT', () {
      expect(
        blockTag(
          kRec: SyncVectors.kRec,
          devId: SyncVectors.devId,
          data: SyncVectors.bootData,
        ),
        SyncVectors.bootTag,
      );
    });

    test('LOSS', () {
      expect(
        blockTag(
          kRec: SyncVectors.kRec,
          devId: SyncVectors.devId,
          data: SyncVectors.lossData,
        ),
        SyncVectors.lossTag,
      );
    });
  });

  test('constantTimeEquals', () {
    expect(constantTimeEquals([1, 2, 3], [1, 2, 3]), isTrue);
    expect(constantTimeEquals([1, 2, 3], [1, 2, 4]), isFalse);
    expect(constantTimeEquals([1, 2], [1, 2, 3]), isFalse);
  });
}
