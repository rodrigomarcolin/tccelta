import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/application/sync/handshake_runner.dart';
import 'package:tccelta_mobile/src/core/errors/sync_failure.dart';
import 'package:tccelta_mobile/src/data/datasources/sync_server_datasource.dart';
import 'package:tccelta_mobile/src/data/repositories/sync_server_repository_impl.dart';
import 'package:tccelta_mobile/src/domain/sync/handshake_messages.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_protocol.dart';
import 'package:tccelta_mobile/src/infra/sync/bench/bench_sync_server.dart';
import 'package:tccelta_mobile/src/infra/sync/bench/bench_sync_server_gateway.dart';
import 'package:tccelta_mobile/src/infra/sync/unavailable_sync_server_gateway.dart';

import '../../support/fake_dongle.dart';
import '../../support/sync_test_blocks.dart';

void main() {
  late FakeDongle dongle;
  late SyncServerRepositoryImpl server;
  late HandshakeRunner runner;

  setUp(() {
    dongle = FakeDongle()..connect();
    server = SyncServerRepositoryImpl(
      SyncServerDatasource(BenchSyncServerGateway(BenchSyncServer())),
    );
    runner = HandshakeRunner(
      dongle: dongle,
      server: server,
      stepTimeout: const Duration(milliseconds: 200),
      confirmTimeout: const Duration(milliseconds: 50),
    );
  });

  Future<dynamic> run({int? generation}) => runner.run(
    benchDevId,
    trigger: HandshakeTrigger.manual,
    generation: generation ?? dongle.generation,
  );

  test('fluxo feliz: tags conferem nos dois lados e a sessão abre', () async {
    dongle.addData(1, 5);
    final outcome = await runner.run(
      benchDevId,
      trigger: HandshakeTrigger.manual,
      generation: dongle.generation,
    );

    expect(dongle.challenges, hasLength(1));
    expect(dongle.acks, hasLength(1));
    expect(dongle.sessionOpen, isTrue);
    expect(AckInfo.fromValue(outcome.ack.value).ackSeq, 0);
    expect(server.token, isNotNull);
    expect(outcome.token.value, server.token!.value);
  });

  test('dongle responde 0x02 (c_S antigo) -> HandshakeRejectedByDongle', () {
    dongle.challengeError = SyncErrorCode.staleChallenge;
    expect(
      run(),
      throwsA(
        isA<HandshakeRejectedByDongle>().having((e) => e.code, 'code', 2),
      ),
    );
  });

  test('resposta com cS diferente é ignorada até o timeout', () async {
    dongle.autoRespond = false;
    final future = run();
    await Future<void>.delayed(const Duration(milliseconds: 20));
    final cS = ChallengeInfo.fromValue(dongle.challenges.single.value).cS;
    dongle.emitResponse(cS + 1);
    await expectLater(future, throwsA(isA<HandshakeTimeout>()));
    expect(dongle.acks, isEmpty);
  });

  test('queda do link no passo 3 -> HandshakeLinkLost', () async {
    dongle.autoRespond = false;
    final future = run();
    await Future<void>.delayed(const Duration(milliseconds: 20));
    dongle.disconnect();
    await expectLater(future, throwsA(isA<HandshakeLinkLost>()));
  });

  test('link com outra geração -> HandshakeLinkLost', () async {
    dongle.autoRespond = false;
    final future = run();
    await Future<void>.delayed(const Duration(milliseconds: 20));
    dongle
      ..disconnect()
      ..connect();
    await expectLater(future, throwsA(isA<HandshakeLinkLost>()));
  });

  test('ACK rejeitado com 0x03 no passo 6 -> HandshakeAckRejected', () {
    dongle.ackError = SyncErrorCode.invalidAckSeq;
    expect(
      run(),
      throwsA(isA<HandshakeAckRejected>().having((e) => e.code, 'code', 3)),
    );
  });

  test('sem confirmação do ACK o handshake ainda conclui', () async {
    dongle.confirmAck = false;
    final outcome = await run();
    expect(outcome, isNotNull);
    expect(dongle.sessionOpen, isTrue);
  });

  test('servidor indisponível sobe como ChallengeUnavailable', () async {
    final stubRunner = HandshakeRunner(
      dongle: dongle,
      server: SyncServerRepositoryImpl(
        SyncServerDatasource(const UnavailableSyncServerGateway()),
      ),
    );
    await expectLater(
      stubRunner.run(
        benchDevId,
        trigger: HandshakeTrigger.manual,
        generation: dongle.generation,
      ),
      throwsA(
        isA<ChallengeUnavailable>().having(
          (e) => e.retryAfter,
          'retryAfter',
          const Duration(minutes: 5),
        ),
      ),
    );
    expect(dongle.challenges, isEmpty);
  });
}
