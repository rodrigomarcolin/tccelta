import 'dart:math';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/domain/sync/handshake_messages.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_frame.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_protocol.dart';
import 'package:tccelta_mobile/src/infra/sync/bench/bench_crypto.dart';
import 'package:tccelta_mobile/src/infra/sync/bench/bench_sync_server.dart';
import 'package:tccelta_mobile/src/services/sync/sync_server_exceptions.dart';
import 'package:tccelta_mobile/src/services/sync/sync_server_gateway.dart';

import '../../../support/sync_known_vectors.dart';
import '../../../support/sync_test_blocks.dart';

void main() {
  late DateTime now;
  late BenchSyncServer server;

  setUp(() {
    now = DateTime.utc(2026, 10, 27, 12);
    server = BenchSyncServer(now: () => now, random: Random(1));
  });

  /// Responde ao [challenge] como o dongle faria.
  SyncFrame respond(
    ChallengeEnvelope challenge, {
    int tail = 1,
    int head = 0,
    bool badTag = false,
    int? nonceByte,
  }) {
    final v = challenge.challenge.value;
    final payload = Uint8List(34);
    payload.buffer.asByteData()
      ..setUint32(0, ChallengeInfo.fromValue(v).cS, Endian.little)
      ..setUint32(20, tail, Endian.little)
      ..setUint32(24, head, Endian.little)
      ..setUint16(28, 3, Endian.little)
      ..setUint32(30, 123456, Endian.little);
    payload.setRange(4, 20, v.sublist(4, 20));
    if (nonceByte != null) payload[4] = nonceByte;
    final tag = handshakeTag(
      kAuth: SyncVectors.kAuth,
      type: 0x02,
      devId: SyncVectors.devId,
      payload: payload,
    );
    if (badTag) tag[0] ^= 1;
    return SyncFrame(
      SyncMessageType.response,
      Uint8List.fromList([...payload, ...tag]),
    );
  }

  HandshakeEnvelope open({int tail = 1, int head = 0}) {
    final ch = server.requestChallenge(benchDevId);
    return server.submitResponse(
      benchDevId,
      respond(ch, tail: tail, head: head),
    );
  }

  AckInfo ackOf(SyncFrame f) => AckInfo.fromValue(f.value);

  group('handshake', () {
    test('desafio: cS cresce e parte do relógio; tag confere', () {
      final a = server.requestChallenge(benchDevId);
      final b = server.requestChallenge(benchDevId);
      final ia = ChallengeInfo.fromValue(a.challenge.value);
      final ib = ChallengeInfo.fromValue(b.challenge.value);
      expect(ia.cS, now.millisecondsSinceEpoch ~/ 1000);
      expect(ib.cS, ia.cS + 1);
      expect(a.challenge.value.length, 45);
      final v = a.challenge.value;
      expect(
        handshakeTag(
          kAuth: SyncVectors.kAuth,
          type: 0x01,
          devId: SyncVectors.devId,
          payload: Uint8List.sublistView(v, 0, 29),
        ),
        v.sublist(29),
      );
    });

    test('resposta válida devolve ACK com tag e token', () {
      final env = open();
      final info = AckInfo.fromValue(env.ack.value);
      expect(info.ackSeq, 0);
      expect(info.anchorFailed, isFalse);
      expect(env.token, startsWith('bench.${benchDevId.hex}.'));
      expect(env.tokenExpiresAt, now.add(const Duration(hours: 1)));
      final v = env.ack.value;
      expect(
        handshakeTag(
          kAuth: SyncVectors.kAuth,
          type: 0x04,
          devId: SyncVectors.devId,
          payload: Uint8List.sublistView(v, 0, 9),
        ),
        v.sublist(9),
      );
    });

    test('tag errada é 422 (tag_invalida)', () {
      final ch = server.requestChallenge(benchDevId);
      expect(
        () => server.submitResponse(benchDevId, respond(ch, badTag: true)),
        throwsA(
          isA<ServerResponseRejected>().having(
            (e) => e.inconsistent,
            'inconsistent',
            isFalse,
          ),
        ),
      );
    });

    test('desafio reapresentado é 409', () {
      final ch = server.requestChallenge(benchDevId);
      final r = respond(ch);
      server.submitResponse(benchDevId, r);
      expect(
        () => server.submitResponse(benchDevId, r),
        throwsA(isA<ServerChallengeRejected>()),
      );
    });

    test('nonce errado e desafio vencido são 409', () {
      final ch = server.requestChallenge(benchDevId);
      expect(
        () => server.submitResponse(benchDevId, respond(ch, nonceByte: 0xEE)),
        throwsA(isA<ServerChallengeRejected>()),
      );
      final ch2 = server.requestChallenge(benchDevId);
      now = now.add(const Duration(seconds: 61));
      expect(
        () => server.submitResponse(benchDevId, respond(ch2)),
        throwsA(isA<ServerChallengeRejected>()),
      );
    });

    test('TAIL > HEAD + 1 é resposta inconsistente', () {
      final ch = server.requestChallenge(benchDevId);
      expect(
        () => server.submitResponse(benchDevId, respond(ch, tail: 9, head: 3)),
        throwsA(
          isA<ServerResponseRejected>().having(
            (e) => e.inconsistent,
            'inconsistent',
            isTrue,
          ),
        ),
      );
    });

    test('regra do TAIL: lacuna OVERFLOW e ackSeq avança', () {
      final env = open(tail: 31, head: 80);
      expect(ackOf(env.ack).ackSeq, 30);
      expect(server.stats.gaps.single.cause, BenchGapCause.overflow);
      expect(server.stats.gaps.single.from, 1);
      expect(server.stats.gaps.single.to, 30);
      // Próximo desafio já informa o ACK avançado.
      final ch = server.requestChallenge(benchDevId);
      expect(ChallengeInfo.fromValue(ch.challenge.value).ackSeq, 30);
    });

    test('âncora com mais de 2 s liga flags_a', () {
      final ch = server.requestChallenge(benchDevId);
      now = now.add(const Duration(seconds: 3));
      final env = server.submitResponse(benchDevId, respond(ch));
      expect(ackOf(env.ack).anchorFailed, isTrue);
    });
  });

  group('lote', () {
    test('em ordem avança o ACK e devolve tag válida', () {
      final env = open();
      final ack = server.postBlocks(env.token, [
        for (var s = 1; s <= 3; s++) dataBlock(s).frame,
      ]);
      expect(ackOf(ack).ackSeq, 3);
      expect(ackOf(ack).cConf, ackOf(env.ack).cConf);
      expect(server.stats.accepted, 3);
    });

    test('LOSS com from = seq avança para to', () {
      final env = open();
      final ack = server.postBlocks(env.token, [
        dataBlock(1).frame,
        lossBlock(2, 5).frame,
        dataBlock(6).frame,
      ]);
      expect(ackOf(ack).ackSeq, 6);
      expect(server.stats.gaps.single.cause, BenchGapCause.loss);
    });

    test('BOOT com from = ack + 1 e to = seq − 1 é aceito', () {
      final env = open();
      server.postBlocks(env.token, [dataBlock(1).frame]);
      final ack = server.postBlocks(env.token, [bootBlock(10, 2, 9).frame]);
      expect(ackOf(ack).ackSeq, 10);
      expect(server.stats.gaps.single.cause, BenchGapCause.boot);
      expect(server.stats.gaps.single.from, 2);
      expect(server.stats.gaps.single.to, 9);
    });

    test('BOOT de faixa vazia em seq = ack + 1 não gera lacuna', () {
      final env = open();
      final ack = server.postBlocks(env.token, [bootBlock(1, 1, 0).frame]);
      expect(ackOf(ack).ackSeq, 1);
      expect(server.stats.gaps, isEmpty);
    });

    test('bloco fora de ordem interrompe e devolve o ACK parcial', () {
      final env = open();
      final ack = server.postBlocks(env.token, [
        dataBlock(1).frame,
        dataBlock(2).frame,
        dataBlock(5).frame,
        dataBlock(6).frame,
      ]);
      expect(ackOf(ack).ackSeq, 2);
      expect(server.stats.rejected, 1);
    });

    test('lote sem nenhum progresso vira lote_sem_progresso com o ACK', () {
      final env = open();
      expect(
        () => server.postBlocks(env.token, [dataBlock(7).frame]),
        throwsA(
          isA<ServerBatchNoProgress>().having(
            (e) => ackOf(e.ack).ackSeq,
            'ackSeq',
            0,
          ),
        ),
      );
    });

    test('seq ≤ ACK é ignorado sem interromper', () {
      final env = open();
      server.postBlocks(env.token, [dataBlock(1).frame, dataBlock(2).frame]);
      final ack = server.postBlocks(env.token, [
        dataBlock(2).frame,
        dataBlock(3).frame,
      ]);
      expect(ackOf(ack).ackSeq, 3);
    });

    test('tag K_rec errada interrompe', () {
      final env = open();
      final bad = Uint8List(32)..[0] = 1;
      expect(
        () => server.postBlocks(env.token, [dataBlock(1, tag: bad).frame]),
        throwsA(isA<ServerBatchNoProgress>()),
      );
    });

    test('tags de 0 B e 64 B são aceitas e contadas como não verificadas', () {
      final env = open();
      final ack = server.postBlocks(env.token, [
        dataBlock(1, tagLength: 0).frame,
        dataBlock(2, tagLength: 64).frame,
      ]);
      expect(ackOf(ack).ackSeq, 2);
      expect(server.stats.unverified, 2);
    });

    test('33 blocos é 429', () {
      final env = open();
      expect(
        () => server.postBlocks(env.token, [
          for (var s = 1; s <= 33; s++) dataBlock(s).frame,
        ]),
        throwsA(isA<ServerRateLimited>()),
      );
    });

    test('token desconhecido e expirado são 401', () {
      final env = open();
      expect(
        () => server.postBlocks('nada', [dataBlock(1).frame]),
        throwsA(isA<ServerUnauthorized>()),
      );
      now = now.add(const Duration(hours: 1, seconds: 1));
      expect(
        () => server.postBlocks(env.token, [dataBlock(1).frame]),
        throwsA(
          isA<ServerUnauthorized>().having(
            (e) => e.code,
            'code',
            'token_expirado',
          ),
        ),
      );
    });

    test('forgetDongles zera o estado mas mantém lastIssuedCs', () {
      final env = open();
      server.postBlocks(env.token, [dataBlock(1).frame]);
      final before = ChallengeInfo.fromValue(
        server.requestChallenge(benchDevId).challenge.value,
      ).cS;
      server.forgetDongles();
      expect(server.stats.accepted, 0);
      final ch = server.requestChallenge(benchDevId);
      final info = ChallengeInfo.fromValue(ch.challenge.value);
      expect(info.ackSeq, 0);
      expect(info.cS, greaterThan(before));
    });
  });
}
