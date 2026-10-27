import 'package:fake_async/fake_async.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/data/datasources/sync_ble_datasource.dart';
import 'package:tccelta_mobile/src/domain/sync/fragment_assembler.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_frame.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_protocol.dart';

import '../../support/fake_ble_channel.dart';
import '../../support/fake_monotonic_clock.dart';
import '../../support/sync_known_vectors.dart';

void main() {
  test('fragmentos viram um frame de resposta', () async {
    final channel = FakeBleChannel();
    final ds = SyncBleDatasource(channel, clock: FakeMonotonicClock());
    final received = <SyncInbound>[];
    ds.inbound.listen(received.add);

    SyncVectors.responseFragments.forEach(channel.emit);
    await Future<void>.delayed(Duration.zero);

    expect(received, hasLength(1));
    final frame = (received.single as InboundFrame).frame;
    expect(frame.type, SyncMessageType.response);
    expect(frame.value.length, 50);
    ds.dispose();
  });

  test('fragmentos e 2 s parados viram InboundIncomplete(timeout)', () {
    fakeAsync((async) {
      final channel = FakeBleChannel();
      final clock = FakeMonotonicClock(source: () => async.elapsed);
      final ds = SyncBleDatasource(channel, clock: clock);
      final received = <SyncInbound>[];
      ds.inbound.listen(received.add);

      channel.emit(SyncVectors.responseFragments.first);
      async.elapse(const Duration(milliseconds: 1400));
      expect(received, isEmpty);
      async.elapse(const Duration(milliseconds: 700));
      expect(received, hasLength(1));
      expect(
        (received.single as InboundIncomplete).kind,
        AssemblyErrorKind.timeout,
      );
      ds.dispose();
    });
  });

  test('mensagem de tipo desconhecido é descartada sem erro', () async {
    final channel = FakeBleChannel();
    final ds = SyncBleDatasource(channel, clock: FakeMonotonicClock());
    final received = <SyncInbound>[];
    ds.inbound.listen(received.add);
    channel.emit([0x80, 0x09, 0x01, 0x00, 0xAA]);
    await Future<void>.delayed(Duration.zero);
    expect(received, isEmpty);
    ds.dispose();
  });

  test('send escreve o TLV exato', () async {
    final channel = FakeBleChannel();
    final ds = SyncBleDatasource(channel, clock: FakeMonotonicClock());
    await ds.send(SyncFrame.batchRequest(62));
    expect(channel.written.single, [0x05, 0x02, 0x00, 0x3E, 0x00]);
    ds.dispose();
  });
}
