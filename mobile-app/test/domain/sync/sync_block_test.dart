import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/domain/sync/handshake_messages.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_block.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_frame.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_protocol.dart';

import '../../support/sync_known_vectors.dart';

SyncFrame _block(List<int> data, List<int> tag) =>
    SyncFrame(SyncMessageType.block, concatBytes([data, tag]));

void main() {
  test('BOOT do vetor expõe a faixa', () {
    final b = SyncBlock.fromFrame(
      _block(SyncVectors.bootData, SyncVectors.bootTag),
    );
    expect(b.marker, MarkerKind.boot);
    expect(b.seq, 192);
    expect(b.bootId, 4);
    expect(b.tMs, 812);
    expect(b.length, 23);
    expect(b.fromSeq, 131);
    expect(b.toSeq, 191);
    expect(b.lastCoveredSeq, 192);
  });

  test('LOSS do vetor avança o cursor até to_seq', () {
    final b = SyncBlock.fromFrame(
      _block(SyncVectors.lossData, SyncVectors.lossTag),
    );
    expect(b.marker, MarkerKind.loss);
    expect(b.seq, 140);
    expect(b.fromSeq, 140);
    expect(b.toSeq, 141);
    expect(b.lastCoveredSeq, 141);
  });

  test('bloco de dados de 23 B cujo tipo é PID não é marcador', () {
    final data = Uint8List.fromList(SyncVectors.bootData)..[12] = 0x0C;
    final b = SyncBlock.fromFrame(_block(data, SyncVectors.bootTag));
    expect(b.marker, MarkerKind.none);
  });

  test('frame curto ou tamanho incoerente lança', () {
    expect(
      () => SyncBlock.fromFrame(
        SyncFrame(SyncMessageType.block, Uint8List(5)),
      ),
      throwsFormatException,
    );
    final data = Uint8List.fromList(SyncVectors.bootData)..[10] = 0xFF;
    expect(
      () => SyncBlock.fromFrame(_block(data, const [])),
      throwsFormatException,
    );
  });

  group('mensagens do handshake', () {
    test('ChallengeInfo', () {
      final c = ChallengeInfo.fromValue(
        concatBytes([SyncVectors.challengePayload, SyncVectors.challengeTag]),
      );
      expect(
        (c.cS, c.ackSeq, c.cConf, c.flags, c.diagMin),
        (7, 100, 6, 0, null),
      );
      final d = ChallengeInfo.fromValue(
        concatBytes([
          SyncVectors.challengeDiagPayload,
          SyncVectors.challengeDiagTag,
        ]),
      );
      expect((d.cS, d.flags, d.diagMin), (8, 1, 30));
    });

    test('ResponseInfo', () {
      final r = ResponseInfo.fromValue(
        concatBytes([SyncVectors.responsePayload, SyncVectors.responseTag]),
      );
      expect(
        (r.cS, r.tailSeq, r.headSeq, r.bootId, r.tMs),
        (7, 65, 130, 3, 123456),
      );
    });

    test('AckInfo', () {
      final a = AckInfo.fromValue(
        concatBytes([SyncVectors.ackPayload, SyncVectors.ackTag]),
      );
      expect((a.ackSeq, a.cConf, a.anchorFailed), (130, 7, false));
      final flagged = Uint8List.fromList(
        concatBytes([SyncVectors.ackPayload, SyncVectors.ackTag]),
      )..[8] = 1;
      expect(AckInfo.fromValue(flagged).anchorFailed, isTrue);
    });

    test('tamanhos errados lançam', () {
      expect(() => AckInfo.fromValue(Uint8List(24)), throwsFormatException);
      expect(
        () => ResponseInfo.fromValue(Uint8List(49)),
        throwsFormatException,
      );
      expect(
        () => ChallengeInfo.fromValue(Uint8List(46)),
        throwsFormatException,
      );
    });
  });
}
