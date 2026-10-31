// Harness com tipos privados e chamadas encadeadas por legibilidade.
// ignore_for_file: library_private_types_in_public_api, cascade_invocations

import 'dart:async';
import 'dart:math';

import 'package:fake_async/fake_async.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/application/sync/handshake_runner.dart';
import 'package:tccelta_mobile/src/application/sync/sync_coordinator.dart';
import 'package:tccelta_mobile/src/core/errors/sync_failure.dart';
import 'package:tccelta_mobile/src/domain/repositories/sync_server_repository.dart';
import 'package:tccelta_mobile/src/domain/sync/retry_backoff.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_frame.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_protocol.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_settings.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_status.dart';
import 'package:tccelta_mobile/src/infra/sync/bench/bench_crypto.dart';

import '../../support/fake_dongle.dart';
import '../../support/fake_monotonic_clock.dart';
import '../../support/fake_sync_server_repository.dart';
import '../../support/fake_sync_settings_repository.dart';
import '../../support/sync_known_vectors.dart';
import '../../support/sync_test_blocks.dart';

/// `Random` fixo em 0,5: o jitter do backoff fica neutro.
class _Half implements Random {
  @override
  bool nextBool() => true;
  @override
  double nextDouble() => 0.5;
  @override
  int nextInt(int max) => max ~/ 2;
}

/// ACK 0x04 com tag válida para as chaves de bancada.
SyncFrame _ack(int ackSeq, {int cConf = 1}) {
  final payload = Uint8List(9);
  payload.buffer.asByteData()
    ..setUint32(0, ackSeq, Endian.little)
    ..setUint32(4, cConf, Endian.little);
  final tag = handshakeTag(
    kAuth: SyncVectors.kAuth,
    type: 0x04,
    devId: SyncVectors.devId,
    payload: payload,
  );
  return SyncFrame(
    SyncMessageType.ack,
    Uint8List.fromList([...payload, ...tag]),
  );
}

class _Harness {
  _Harness(this.async, {SyncSettings settings = const SyncSettings()}) {
    clock = FakeMonotonicClock(source: () => async.elapsed);
    settingsRepo = FakeSyncSettingsRepository(settings);
    coordinator = SyncCoordinator(
      dongle: dongle,
      server: server,
      settings: settingsRepo,
      handshake: HandshakeRunner(dongle: dongle, server: server),
      clock: clock,
      backoff: RetryBackoff(random: _Half()),
    )..start();
    async.flushMicrotasks();
  }

  final FakeAsync async;
  final FakeDongle dongle = FakeDongle();
  final FakeSyncServerRepository server = FakeSyncServerRepository();
  late final FakeMonotonicClock clock;
  late final FakeSyncSettingsRepository settingsRepo;
  late final SyncCoordinator coordinator;

  SyncStatus get status => coordinator.current;

  void pump() => async.elapse(Duration.zero);

  void elapse(Duration d) => async.elapse(d);

  /// Sobe o link e entrega o aviso inicial (abre a sessão pelo handshake).
  void open() {
    dongle.connect();
    pump();
    dongle.notice();
    pump();
  }
}

void withHarness(
  String name,
  void Function(_Harness h) body, {
  SyncSettings settings = const SyncSettings(),
}) {
  test(name, () {
    fakeAsync((async) {
      final h = _Harness(async, settings: settings);
      body(h);
      unawaited(h.coordinator.dispose());
      async.flushMicrotasks();
    });
  });
}

void main() {
  // Sem o *throttle* do debugPrint (que usa Timer e não roda no fakeAsync).
  debugPrint = (message, {wrapWidth}) {};

  withHarness('aviso com bit 0 -> handshake -> ACK escrito -> pedido de lote', (
    h,
  ) {
    h.dongle.addData(1, 10);
    h.open();

    expect(h.server.log.take(2), ['challenge', 'response']);
    expect(h.dongle.acks, hasLength(1));
    expect(h.dongle.sessionOpen, isTrue);
    expect(h.dongle.batches, [10]);
    expect(h.status.phase, SyncPhase.open);
    expect(h.status.buffered, 10);
    expect(h.status.lastRead, 10);
    expect(h.status.headSeq, 10);
  });

  withHarness('blocos ficam no buffer até T_lat; POST confirma e libera', (h) {
    h.dongle.addData(1, 10);
    h.open();

    h.elapse(const Duration(seconds: 59));
    expect(h.server.posted, isEmpty);
    expect(h.status.buffered, 10);

    h.elapse(const Duration(seconds: 2));
    expect(h.server.posted, hasLength(1));
    expect(h.server.posted.single, hasLength(10));
    expect(h.status.appAck, 10);
    expect(h.status.buffered, 0);
    expect(h.dongle.ackLocal, 10);
    expect(h.dongle.acks.last.value[0], 10);
    expect(h.status.postsOk, 1);
  });

  withHarness('40 blocos: POST de 32 imediato e outro de 8 após T_lat', (h) {
    h.dongle.addData(1, 40);
    h.open();

    expect(h.server.posted.map((p) => p.length), [32]);
    h.elapse(const Duration(seconds: 61));
    expect(h.server.posted.map((p) => p.length), [32, 8]);
    expect(h.status.appAck, 40);
  });

  withHarness(
    'B_max cheio para de pedir; ACK libera e retoma',
    (h) {
      h.dongle.addData(1, 25);
      h.open();

      expect(h.dongle.batches, [10]);
      h.elapse(const Duration(seconds: 30));
      expect(h.dongle.batches, [10]);

      h.elapse(const Duration(seconds: 31));
      expect(h.server.posted.first, hasLength(10));
      expect(h.dongle.batches.length, greaterThan(1));
      expect(h.dongle.batches[1], 10);
      expect(h.status.lastRead, 20);
    },
    settings: const SyncSettings(bufferCapacity: 10),
  );

  withHarness('flash alta: POST sem esperar T_lat', (h) {
    h.dongle.addData(1, 3);
    h.open();
    expect(h.server.posted, isEmpty);

    h.dongle.notice(flags: 0x04);
    h.pump();

    expect(h.server.posted, hasLength(1));
    expect(h.server.posted.single, hasLength(3));
  });

  group('ressincronização', () {
    withHarness('fragmento perdido: um único 0x04 e o bloco ACK+1 encerra', (
      h,
    ) {
      h.dongle
        ..addData(1, 20)
        ..swallowOnce.add(5);
      h.open();

      expect(h.dongle.errors, [SyncErrorCode.incompleteMessage]);
      expect(h.dongle.batches, [20, 20]);
      expect(h.status.phase, SyncPhase.open);
      expect(h.status.lastRead, 20);
      expect(h.status.buffered, 20);
    });

    withHarness('LOSS seguido de b+1 é aceito', (h) {
      h.dongle
        ..addData(1, 2)
        ..addBlock(lossBlock(3, 5))
        ..addData(6, 7);
      h.open();

      expect(h.dongle.errors, isEmpty);
      expect(h.status.lastRead, 7);
      expect(h.status.buffered, 5);
    });

    withHarness('BOOT com lacuna é aceito', (h) {
      h.dongle
        ..addData(1, 2)
        ..addBlock(bootBlock(10, 3, 9));
      h.open();

      expect(h.dongle.errors, isEmpty);
      expect(h.status.lastRead, 10);
      expect(h.status.buffered, 3);
    });

    withHarness('salto sem marcador vira 0x04', (h) {
      h.dongle.addData(1, 3);
      h.open();
      h.dongle
        ..addData(4, 9)
        ..emitBlock(dataBlock(9));
      h.pump();

      expect(h.dongle.errors, [SyncErrorCode.incompleteMessage]);
      expect(h.status.phase, isIn([SyncPhase.resyncing, SyncPhase.open]));
    });

    withHarness('TransportIncomplete vira 0x04 uma só vez', (h) {
      h.dongle.addData(1, 3);
      h.open();
      h.dongle
        ..emitIncomplete()
        ..emitIncomplete();
      h.pump();

      expect(h.dongle.errors, [SyncErrorCode.incompleteMessage]);
    });

    withHarness('lote interrompido duas vezes no mesmo ACK vira 0x07', (h) {
      h.dongle.addData(1, 40);
      h.server.scripted
        ..add(BlocksAccepted(_ack(2)))
        ..add(BlocksAccepted(_ack(2)));
      h.open();

      expect(h.server.posted.length, greaterThanOrEqualTo(2));
      expect(h.dongle.errors, contains(SyncErrorCode.resendFromBoundary));
    });
  });

  group('erros do dongle', () {
    withHarness('0x05 reescreve o ACK de abertura uma vez, depois handshake', (
      h,
    ) {
      h.dongle.addData(1, 3);
      h.open();
      expect(h.dongle.acks, hasLength(1));

      h.dongle.emitError(SyncErrorCode.noSession);
      h.pump();
      expect(h.dongle.acks, hasLength(2));
      expect(h.dongle.acks.first, h.dongle.acks.last);
      expect(h.server.log.where((e) => e == 'challenge'), hasLength(1));

      h.dongle.emitError(SyncErrorCode.noSession);
      h.pump();
      expect(h.server.log.where((e) => e == 'challenge'), hasLength(2));
    });

    withHarness('0x03 só é registrado', (h) {
      h.dongle.addData(1, 3);
      h.open();
      h.dongle.emitError(SyncErrorCode.invalidAckSeq);
      h.pump();
      expect(h.server.log.where((e) => e == 'challenge'), hasLength(1));
      expect(h.status.phase, SyncPhase.open);
    });
  });

  group('respostas do servidor ao POST', () {
    withHarness('401 -> handshake e reenvio', (h) {
      h.dongle.addData(1, 40);
      h.server.scripted.add(const Unauthorized());
      h.open();
      h.elapse(const Duration(seconds: 1));

      expect(h.server.log.where((e) => e == 'challenge').length, 2);
      expect(h.server.posted.length, 2);
      expect(h.status.appAck, 32);
    });

    withHarness('429 espera Retry-After e reduz o lote', (h) {
      h.dongle.addData(1, 40);
      h.server.scripted.add(const RateLimited(Duration(seconds: 5)));
      h.open();
      expect(h.server.posted.map((p) => p.length), [32]);

      h.elapse(const Duration(seconds: 4));
      expect(h.server.posted, hasLength(1));
      h.elapse(const Duration(seconds: 2));
      expect(h.server.posted.map((p) => p.length), [32, 16]);
    });

    withHarness('503 faz backoff de 1, 2 e 4 s', (h) {
      h.dongle.addData(1, 40);
      h.server.scripted
        ..add(const PostUnavailable(null))
        ..add(const PostUnavailable(null))
        ..add(const PostUnavailable(null));
      h.open();
      expect(h.server.posted, hasLength(1));

      h.elapse(const Duration(milliseconds: 500));
      expect(h.server.posted, hasLength(1));
      h.elapse(const Duration(seconds: 1));
      expect(h.server.posted, hasLength(2));
      h.elapse(const Duration(seconds: 1));
      expect(h.server.posted, hasLength(2));
      h.elapse(const Duration(seconds: 1));
      expect(h.server.posted, hasLength(3));
      h.elapse(const Duration(seconds: 3));
      expect(h.server.posted, hasLength(3));
      h.elapse(const Duration(seconds: 1));
      expect(h.server.posted, predicate<List<Object?>>((l) => l.length >= 4));
      expect(h.status.appAck, isNotNull);
      expect(h.status.postsFailed, 3);
    });

    withHarness('lote aceito depois de 503 apaga a próxima tentativa', (h) {
      h.dongle.addData(1, 40);
      h.server.scripted.add(const PostUnavailable(null));
      h.open();
      expect(h.status.nextAttemptAt, isNotNull);

      h.elapse(const Duration(seconds: 2));
      expect(h.status.appAck, isNotNull);
      expect(h.status.phase, SyncPhase.open);
      expect(h.status.nextAttemptAt, isNull);
    });

    withHarness('dongle bloqueado para o coordenador', (h) {
      h.dongle.addData(1, 40);
      h.server.scripted.add(const DongleBlocked('dongle_inativo'));
      h.open();
      h.elapse(const Duration(minutes: 5));

      expect(h.status.phase, SyncPhase.stopped);
      expect(h.server.posted, hasLength(1));
    });

    withHarness('400 vira 0x07', (h) {
      h.dongle.addData(1, 40);
      h.server.scripted.add(const MalformedRequest('lote_malformado'));
      h.open();
      expect(h.dongle.errors, contains(SyncErrorCode.resendFromBoundary));
    });
  });

  withHarness('reconexão: pendentes descartados, POST em voo conclui depois', (
    h,
  ) {
    h.dongle.addData(1, 40);
    h.server.postGate = Completer<void>();
    h.open();
    expect(h.server.posted.map((p) => p.length), [32]);

    h.dongle
      ..disconnect()
      ..connect();
    h.pump();
    expect(h.status.buffered, 32);
    expect(h.status.phase, SyncPhase.waitingNotice);

    h.server.postGate!.complete();
    h.server.postGate = null;
    h.pump();
    expect(h.status.appAck, 32);

    h.dongle.notice();
    h.pump();
    expect(h.status.phase, SyncPhase.open);
    expect(h.dongle.ackLocal, 32);
    expect(h.status.lastRead, greaterThanOrEqualTo(32));
  });

  withHarness('token a menos de 5 min: handshake antes do POST', (h) {
    h.dongle.addData(1, 5);
    h.open();
    expect(h.server.log.where((e) => e == 'challenge'), hasLength(1));

    h.server.forceNeedsHandshake = true;
    h.dongle.addData(6, 40);
    h.dongle.notice();
    h.pump();
    h.elapse(const Duration(seconds: 1));

    expect(h.server.log.where((e) => e == 'challenge'), hasLength(2));
    expect(h.server.posted, isNotEmpty);
    expect(h.server.log.indexOf('post'), greaterThan(2));
  });

  withHarness('link cai e volta no handshake: resultado antigo é descartado', (
    h,
  ) {
    h.dongle.addData(1, 5);
    h.server.submitGate = Completer<void>();
    h.open();
    expect(h.server.log, ['challenge', 'response']);

    h.dongle
      ..disconnect()
      ..connect();
    h.pump();
    h.server.submitGate!.complete();
    h.server.submitGate = null;
    h.pump();
    expect(h.dongle.acks, isEmpty);
    expect(h.status.phase, SyncPhase.waitingNotice);

    h.dongle.notice();
    h.pump();
    expect(h.dongle.acks, hasLength(1));
    expect(h.status.phase, SyncPhase.open);
  });

  withHarness('aviso com bit 0 e blocos pendentes: POST antes do handshake', (
    h,
  ) {
    h.dongle.addData(1, 10);
    h.open();
    expect(h.status.buffered, 10);
    h.server.log.clear();

    h.dongle.notice(flags: 0x05);
    h.pump();

    expect(h.server.log, ['post', 'challenge', 'response']);
    expect(h.status.appAck, 10);
  });

  group('servidor indisponível', () {
    withHarness('handshake pendente com espera crescente e sem lote', (h) {
      h.server.challengeFailure = const ChallengeUnavailable(
        'Servidor indisponível',
      );
      h.dongle.addData(1, 5);
      h.open();

      expect(h.status.phase, SyncPhase.handshakePending);
      expect(h.status.nextAttemptAt, isNotNull);
      expect(h.dongle.batches, isEmpty);
      expect(h.server.log, ['challenge']);

      h.elapse(const Duration(milliseconds: 1500));
      expect(h.server.log.length, 2);
      h.elapse(const Duration(seconds: 2));
      expect(h.server.log.length, 3);
      expect(h.dongle.batches, isEmpty);

      h.server.challengeFailure = null;
      h.elapse(const Duration(seconds: 5));
      expect(h.status.phase, SyncPhase.open);
      expect(h.dongle.batches, isNotEmpty);
    });

    withHarness('Retry-After longo prevalece sobre o backoff', (h) {
      h.server.challengeFailure = const ChallengeUnavailable(
        'x',
        retryAfter: Duration(minutes: 5),
      );
      h.open();
      h.elapse(const Duration(minutes: 4));
      expect(h.server.log, ['challenge']);
      h.elapse(const Duration(minutes: 2));
      expect(h.server.log.length, 2);
    });
  });

  withHarness('T_lat efetivo é o menor entre o local e o do servidor', (h) {
    h.server.suggestedLatency = const Duration(seconds: 20);
    h.dongle.addData(1, 3);
    h.open();

    expect(h.status.effectiveLatency, const Duration(seconds: 20));
    expect(h.status.latencySource, LatencySource.server);
    h.elapse(const Duration(seconds: 19));
    expect(h.server.posted, isEmpty);
    h.elapse(const Duration(seconds: 2));
    expect(h.server.posted, hasLength(1));
  });

  withHarness('sugestão do servidor acima do local não vale', (h) {
    h.server.suggestedLatency = const Duration(seconds: 300);
    h.dongle.addData(1, 3);
    h.open();
    expect(h.status.effectiveLatency, const Duration(seconds: 60));
    expect(h.status.latencySource, LatencySource.local);
  });

  withHarness('mudar T_lat vale na próxima avaliação', (h) {
    h.dongle.addData(1, 3);
    h.open();
    unawaited(
      h.settingsRepo.save(const SyncSettings(latency: Duration(seconds: 10))),
    );
    h.pump();
    h.elapse(const Duration(seconds: 11));
    expect(h.server.posted, hasLength(1));
  });

  withHarness('forceHandshake refaz o handshake', (h) {
    h.dongle.addData(1, 3);
    h.open();
    unawaited(h.coordinator.forceHandshake());
    h.pump();
    expect(h.server.handshakes, 2);
  });

  withHarness('sem link o estado é offline; ao subir, aguardando aviso', (h) {
    expect(h.status.phase, SyncPhase.offline);
    h.dongle.connect();
    h.pump();
    expect(h.status.phase, SyncPhase.waitingNotice);
    h.dongle.disconnect();
    h.pump();
    expect(h.status.phase, SyncPhase.offline);
  });
}
