import 'dart:async';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';
import 'package:tccelta_mobile/src/domain/repositories/dongle_sync_repository.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_block.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_frame.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_protocol.dart';
import 'package:tccelta_mobile/src/infra/sync/inject/injecting_dongle_sync_repository.dart';

class _FakeDongle implements DongleSyncRepository {
  final StreamController<DongleSyncEvent> _ctrl =
      StreamController<DongleSyncEvent>.broadcast();
  final List<String> calls = [];

  void emit(DongleSyncEvent e) => _ctrl.add(e);

  @override
  Stream<DongleSyncEvent> get events => _ctrl.stream;
  @override
  bool get isLinkUp => true;
  @override
  int get linkGeneration => 3;
  @override
  int get mtu => 185;
  @override
  Future<void> sendChallenge(SyncFrame challenge) async {
    calls.add('challenge');
  }

  @override
  Future<void> sendAck(SyncFrame ack) async {
    calls.add('ack');
  }

  @override
  Future<void> requestBatch(int k) async {
    calls.add('batch:$k');
  }

  @override
  Future<void> sendError(SyncErrorCode code) async {
    calls.add('error:${code.code}');
  }

  Future<void> close() => _ctrl.close();
}

SyncBlock _dataBlock({required int seq}) {
  const size = kBlockHeaderSize + 5;
  final value = Uint8List(size);
  value.buffer.asByteData()
    ..setUint32(0, seq, Endian.little)
    ..setUint16(4, 1, Endian.little)
    ..setUint32(6, 0, Endian.little)
    ..setUint16(10, size, Endian.little);
  value.setRange(kBlockHeaderSize, size, const [0x0C, 0, 0, 0x10, 0x00]);
  return SyncBlock.fromFrame(SyncFrame(SyncMessageType.block, value));
}

SyncBlock _bootBlock({required int seq}) {
  const size = 23;
  final value = Uint8List(size);
  value.buffer.asByteData()
    ..setUint32(0, seq, Endian.little)
    ..setUint16(4, 1, Endian.little)
    ..setUint32(6, 0, Endian.little)
    ..setUint16(10, size, Endian.little)
    ..setUint32(15, 1, Endian.little)
    ..setUint32(19, seq, Endian.little);
  value[12] = 0xFE; // marcador boot
  return SyncBlock.fromFrame(SyncFrame(SyncMessageType.block, value));
}

void main() {
  group('InjectingDongleSyncRepository', () {
    late _FakeDongle fake;
    Map<Obd2Pid, double>? sample;
    late InjectingDongleSyncRepository deco;

    setUp(() {
      fake = _FakeDongle();
      sample = null;
      deco = InjectingDongleSyncRepository(fake, () => sample);
    });

    tearDown(() async {
      await deco.dispose();
      await fake.close();
    });

    Future<BlockReceived> emitBlock(SyncBlock block) async {
      final events = <DongleSyncEvent>[];
      final sub = deco.events.listen(events.add);
      fake.emit(BlockReceived(block));
      await pumpEventQueue();
      await sub.cancel();
      return events.single as BlockReceived;
    }

    test('reescreve o bloco de dados quando há amostra', () async {
      sample = {Obd2Pid.rpm: 3000, Obd2Pid.speed: 120};
      final real = _dataBlock(seq: 10);
      final out = await emitBlock(real);
      expect(out.block.seq, 10);
      expect(identical(out.block.frame, real.frame), isFalse);
    });

    test('injeção desligada: bloco passa intacto', () async {
      sample = null;
      final real = _dataBlock(seq: 11);
      final out = await emitBlock(real);
      expect(identical(out.block.frame, real.frame), isTrue);
    });

    test('marcador boot passa intacto mesmo com amostra', () async {
      sample = {Obd2Pid.rpm: 3000};
      final boot = _bootBlock(seq: 12);
      final out = await emitBlock(boot);
      expect(out.block.marker, MarkerKind.boot);
      expect(identical(out.block.frame, boot.frame), isTrue);
    });

    test('eventos que não são bloco passam intactos', () async {
      sample = {Obd2Pid.rpm: 3000};
      final events = <DongleSyncEvent>[];
      final sub = deco.events.listen(events.add);
      const original = DongleErrorReceived(SyncErrorCode.noSession);
      fake.emit(original);
      await pumpEventQueue();
      await sub.cancel();
      expect(identical(events.single, original), isTrue);
    });

    test('delega as escritas e os getters ao repo interno', () async {
      expect(deco.isLinkUp, isTrue);
      expect(deco.linkGeneration, 3);
      expect(deco.mtu, 185);
      await deco.requestBatch(5);
      await deco.sendError(SyncErrorCode.resendFromBoundary);
      expect(fake.calls, containsAll(<String>['batch:5', 'error:7']));
    });
  });
}
