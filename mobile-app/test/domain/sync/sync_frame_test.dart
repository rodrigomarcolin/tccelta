import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/domain/sync/dev_id.dart';
import 'package:tccelta_mobile/src/domain/sync/new_block_notice.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_frame.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_protocol.dart';

import '../../support/sync_known_vectors.dart';

void main() {
  test('desafio do vetor: TLV ida e volta', () {
    final tlv = hexBytes(
      '01 2D00 07000000 ${SyncVectors.nonce} 64000000 06000000 00 '
      '5E08174F9D21CAA313D52A39DCD31943',
    );
    final frame = SyncFrame.parseTlv(tlv);
    expect(frame.type, SyncMessageType.challenge);
    expect(frame.value.length, 45);
    expect(frame.toTlv(), tlv);
  });

  test('ACK do vetor e pedido de lote serializam em little-endian', () {
    final ack = SyncFrame(
      SyncMessageType.ack,
      concatBytes([SyncVectors.ackPayload, SyncVectors.ackTag]),
    );
    expect(
      ack.toTlv(),
      hexBytes('04 1900 82000000 07000000 00 84EB2C1203B2720E492DB89E04F1FAE7'),
    );
    expect(SyncFrame.batchRequest(62).toTlv(), hexBytes('05 0200 3E00'));
    expect(
      SyncFrame.error(SyncErrorCode.incompleteMessage).toTlv(),
      hexBytes('06 0100 04'),
    );
  });

  test('aviso do vetor é lido', () {
    final frame = SyncFrame.parseTlv(
      concatBytes([
        [0x07, 0x0D, 0x00],
        SyncVectors.noticeValue,
      ]),
    );
    final notice = NewBlockNotice.fromValue(frame.value);
    expect(notice.headSeq, 130);
    expect(notice.flags.noHandshakeConnection, isTrue);
    expect(notice.flags.noHandshakeBoot, isTrue);
    expect(notice.flags.flashHigh, isFalse);
    expect(notice.devId, DevId.fromHex('544343454C544101'));
  });

  test('L incoerente, curto ou tipo desconhecido lançam', () {
    expect(
      () => SyncFrame.parseTlv(hexBytes('04 1900 00')),
      throwsFormatException,
    );
    expect(() => SyncFrame.parseTlv(hexBytes('04 19')), throwsFormatException);
    expect(
      () => SyncFrame.parseTlv(hexBytes('09 0100 00')),
      throwsFormatException,
    );
    expect(
      () => NewBlockNotice.fromValue(Uint8List(12)),
      throwsFormatException,
    );
  });

  test('batchRequest rejeita 0 e acima de 65535', () {
    expect(() => SyncFrame.batchRequest(0), throwsRangeError);
    expect(() => SyncFrame.batchRequest(65536), throwsRangeError);
  });
}
