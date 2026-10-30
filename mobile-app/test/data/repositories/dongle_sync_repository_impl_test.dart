import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/core/errors/sync_failure.dart';
import 'package:tccelta_mobile/src/data/datasources/dongle_datasource.dart';
import 'package:tccelta_mobile/src/data/repositories/dongle_repository_impl.dart';
import 'package:tccelta_mobile/src/data/repositories/dongle_sync_repository_impl.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';
import 'package:tccelta_mobile/src/domain/repositories/dongle_sync_repository.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_frame.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_protocol.dart';

import '../../support/fake_ble_channel.dart';
import '../../support/fake_ble_service.dart';
import '../../support/sync_known_vectors.dart';

List<int> _noticeFragment() => [
  0x80,
  0x07,
  0x0D,
  0x00,
  ...SyncVectors.noticeValue,
];

Future<void> _settle() =>
    Future<void>.delayed(const Duration(milliseconds: 30));

void main() {
  test(
    'aviso enviado antes de ready chega depois de SyncLinkChanged(up)',
    () async {
      final channel = FakeBleChannel();
      final ble = FakeBleService(
        channels: {SyncConfig.service: channel},
        onPhase: (phase) {
          if (phase == BleConnectionPhase.enablingNotify) {
            channel.emit(_noticeFragment());
          }
        },
      );
      final dongle = DongleRepositoryImpl(DongleDatasource(ble));
      final sync = DongleSyncRepositoryImpl(dongle);
      final events = <DongleSyncEvent>[];
      sync.events.listen(events.add);

      await dongle.connect('AA');
      await _settle();

      expect(events, hasLength(2));
      expect(events[0], isA<SyncLinkChanged>());
      expect((events[0] as SyncLinkChanged).up, isTrue);
      expect((events[0] as SyncLinkChanged).generation, 1);
      expect((events[1] as NoticeReceived).notice.headSeq, 130);
      expect(sync.isLinkUp, isTrue);
      expect(sync.mtu, 247);
      sync.dispose();
    },
  );

  test('reconnecting -> ready emite down/up com geração nova', () async {
    final channel = FakeBleChannel();
    final ble = FakeBleService(channels: {SyncConfig.service: channel});
    final dongle = DongleRepositoryImpl(DongleDatasource(ble));
    final sync = DongleSyncRepositoryImpl(dongle);
    final events = <DongleSyncEvent>[];
    sync.events.listen(events.add);

    await dongle.connect('AA');
    await _settle();
    final conn = dongle.connection! as FakeBleConnection
      ..emitPhase(BleConnectionPhase.reconnecting);
    await _settle();
    conn.emitPhase(BleConnectionPhase.ready);
    await _settle();

    final changes = events.whereType<SyncLinkChanged>().toList();
    expect(changes.map((e) => (e.up, e.generation)), [
      (true, 1),
      (false, 1),
      (true, 2),
    ]);
    sync.dispose();
  });

  test('blocos, erros e respostas viram eventos tipados', () async {
    final channel = FakeBleChannel();
    final ble = FakeBleService(channels: {SyncConfig.service: channel});
    final dongle = DongleRepositoryImpl(DongleDatasource(ble));
    final sync = DongleSyncRepositoryImpl(dongle);
    final events = <DongleSyncEvent>[];
    sync.events.listen(events.add);
    await dongle.connect('AA');
    await _settle();

    channel
      ..emit([0x80, 0x06, 0x01, 0x00, 0x05])
      ..emit([0x80, 0x06, 0x01, 0x00, 0x7F]); // código desconhecido
    SyncVectors.responseFragments.forEach(channel.emit);
    await _settle();

    final typed = events.where((e) => e is! SyncLinkChanged).toList();
    expect(typed, hasLength(2));
    expect(
      (typed[0] as DongleErrorReceived).code,
      SyncErrorCode.noSession,
    );
    expect((typed[1] as ResponseReceived).info.tailSeq, 65);
    sync.dispose();
  });

  test('envio valida tipos e escreve no canal', () async {
    final channel = FakeBleChannel();
    final ble = FakeBleService(channels: {SyncConfig.service: channel});
    final dongle = DongleRepositoryImpl(DongleDatasource(ble));
    final sync = DongleSyncRepositoryImpl(dongle);
    await dongle.connect('AA');
    await _settle();

    await sync.requestBatch(10);
    await sync.sendError(SyncErrorCode.incompleteMessage);
    expect(channel.written, [
      [0x05, 0x02, 0x00, 0x0A, 0x00],
      [0x06, 0x01, 0x00, 0x04],
    ]);
    expect(
      () => sync.sendError(SyncErrorCode.noSession),
      throwsArgumentError,
    );
    expect(
      () => sync.sendAck(SyncFrame.batchRequest(1)),
      throwsArgumentError,
    );
    channel.writeError = Exception('falhou');
    await expectLater(sync.requestBatch(1), throwsA(isA<SyncLinkFailure>()));
    sync.dispose();
  });

  test('canal ausente: link nunca sobe e o NUS segue funcionando', () async {
    final ble = FakeBleService();
    final dongle = DongleRepositoryImpl(DongleDatasource(ble));
    final sync = DongleSyncRepositoryImpl(dongle);
    final events = <DongleSyncEvent>[];
    sync.events.listen(events.add);

    await dongle.connect('AA');
    await _settle();

    expect(dongle.isReady, isTrue);
    expect(sync.isLinkUp, isFalse);
    expect(events, isEmpty);
    await expectLater(sync.requestBatch(1), throwsA(isA<SyncLinkFailure>()));
    sync.dispose();
  });
}
