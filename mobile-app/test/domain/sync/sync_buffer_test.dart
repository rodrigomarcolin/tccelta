import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/domain/sync/send_policy.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_block.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_buffer.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_frame.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_protocol.dart';

BufferedBlock b(int seq, {Duration readAt = Duration.zero}) => BufferedBlock(
  block: SyncBlock(
    seq: seq,
    bootId: 1,
    tMs: 0,
    length: 17,
    marker: MarkerKind.none,
    fromSeq: 0,
    toSeq: 0,
    frame: SyncFrame(SyncMessageType.block, Uint8List(0)),
  ),
  readAt: readAt,
);

void main() {
  group('SyncBuffer', () {
    test('capacidade e duplicata', () {
      final buf = SyncBuffer(capacity: 2);
      expect(buf.add(b(1)), isTrue);
      expect(buf.add(b(1)), isFalse);
      expect(buf.add(b(2)), isTrue);
      expect(buf.add(b(3)), isFalse);
      expect(buf.freeSlots, 0);
      expect(buf.length, 2);
    });

    test('takeForPost em ordem, marca em voo e releaseInFlight devolve', () {
      final buf = SyncBuffer(capacity: 10);
      for (final s in [3, 1, 2]) {
        buf.add(b(s));
      }
      final lot = buf.takeForPost(2);
      expect(lot.map((e) => e.block.seq), [1, 2]);
      expect(buf.inFlightCount, 2);
      expect(buf.pendingCount, 1);
      expect(buf.oldestPending!.block.seq, 3);
      buf.releaseInFlight();
      expect(buf.inFlightCount, 0);
      expect(buf.oldestPending!.block.seq, 1);
    });

    test('removeUpTo remove também os em voo', () {
      final buf = SyncBuffer(capacity: 10);
      for (var s = 1; s <= 4; s++) {
        buf.add(b(s));
      }
      buf
        ..takeForPost(2)
        ..removeUpTo(3);
      expect(buf.length, 1);
      expect(buf.inFlightCount, 0);
    });

    test('dropPending preserva os em voo', () {
      final buf = SyncBuffer(capacity: 10);
      for (var s = 1; s <= 4; s++) {
        buf.add(b(s));
      }
      buf
        ..takeForPost(2)
        ..dropPending();
      expect(buf.length, 2);
      expect(buf.inFlightCount, 2);
    });

    test('reduzir a capacidade não descarta', () {
      final buf = SyncBuffer(capacity: 10);
      for (var s = 1; s <= 5; s++) {
        buf.add(b(s));
      }
      buf.capacity = 2;
      expect(buf.length, 5);
      expect(buf.freeSlots, 0);
      expect(buf.add(b(6)), isFalse);
    });
  });

  group('SendPolicy', () {
    const policy = SendPolicy(tLat: Duration(seconds: 60));
    SyncBuffer buffer(int n, {Duration readAt = Duration.zero}) {
      final buf = SyncBuffer(capacity: 100);
      for (var s = 1; s <= n; s++) {
        buf.add(b(s, readAt: readAt));
      }
      return buf;
    }

    int lot(
      SyncBuffer buf, {
      required Duration now,
      bool inFlight = false,
      bool flashHigh = false,
      Duration? notBefore,
      int limit = 32,
    }) => policy.lotSize(
      now: now,
      buffer: buf,
      postInFlight: inFlight,
      flashHigh: flashHigh,
      notBefore: notBefore,
      currentLotLimit: limit,
    );

    test('T_lat vencido', () {
      final buf = buffer(5);
      expect(lot(buf, now: const Duration(seconds: 59)), 0);
      expect(lot(buf, now: const Duration(seconds: 60)), 5);
    });

    test('32 pendentes disparam, com teto de 32', () {
      expect(lot(buffer(40), now: Duration.zero), 32);
      expect(lot(buffer(31), now: Duration.zero), 0);
    });

    test('flashHigh dispara sem esperar', () {
      expect(lot(buffer(3), now: Duration.zero, flashHigh: true), 3);
    });

    test('notBefore e postInFlight bloqueiam', () {
      final buf = buffer(40);
      expect(
        lot(buf, now: Duration.zero, notBefore: const Duration(seconds: 5)),
        0,
      );
      expect(lot(buf, now: Duration.zero, inFlight: true), 0);
      expect(
        lot(
          buf,
          now: const Duration(seconds: 5),
          notBefore: const Duration(seconds: 5),
        ),
        32,
      );
    });

    test('limite reduzido após 429/5xx', () {
      expect(lot(buffer(40), now: Duration.zero, limit: 8), 8);
    });

    test('buffer vazio não posta', () {
      expect(lot(buffer(0), now: const Duration(hours: 1)), 0);
    });
  });
}
