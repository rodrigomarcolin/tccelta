import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/core/errors/sync_failure.dart';
import 'package:tccelta_mobile/src/data/datasources/sync_server_datasource.dart';
import 'package:tccelta_mobile/src/data/repositories/sync_server_repository_impl.dart';
import 'package:tccelta_mobile/src/domain/repositories/sync_server_repository.dart';
import 'package:tccelta_mobile/src/domain/sync/dev_id.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_frame.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_protocol.dart';
import 'package:tccelta_mobile/src/infra/sync/unavailable_sync_server_gateway.dart';
import 'package:tccelta_mobile/src/services/sync/sync_server_exceptions.dart';
import 'package:tccelta_mobile/src/services/sync/sync_server_gateway.dart';

import '../../support/sync_known_vectors.dart';

SyncFrame _ack() => SyncFrame(SyncMessageType.ack, Uint8List(25));
SyncFrame _challenge([int n = 45]) =>
    SyncFrame(SyncMessageType.challenge, Uint8List(n));

/// Gateway roteirizável: devolve [challenge]/[ack] ou lança [error].
class _ScriptedGateway implements SyncServerGateway {
  SyncFrame challenge = _challenge();
  SyncFrame ack = _ack();
  Exception? error;
  DateTime expiresAt = DateTime.utc(2030);
  Duration? suggested;

  @override
  String get mode => 'scripted';

  @override
  Future<ChallengeEnvelope> requestChallenge(DevId devId) async {
    if (error != null) throw error!;
    return ChallengeEnvelope(challenge: challenge);
  }

  @override
  Future<HandshakeEnvelope> submitResponse(
    DevId devId,
    SyncFrame response,
  ) async {
    if (error != null) throw error!;
    return HandshakeEnvelope(
      ack: ack,
      token: 'tok',
      tokenExpiresAt: expiresAt,
      suggestedLatency: suggested,
    );
  }

  @override
  Future<SyncFrame> postBlocks(String token, List<SyncFrame> blocks) async {
    if (error != null) throw error!;
    return ack;
  }

  @override
  Future<SyncFrame?> latestConfirmation(DevId devId, String token) async =>
      null;
}

void main() {
  late _ScriptedGateway gateway;
  late SyncServerRepositoryImpl repo;
  final devId = DevId(SyncVectors.devId);
  final response = SyncFrame(SyncMessageType.response, Uint8List(50));

  setUp(() {
    gateway = _ScriptedGateway();
    repo = SyncServerRepositoryImpl(SyncServerDatasource(gateway));
  });

  test('stub indisponível sempre falha como ServerUnavailable', () async {
    const stub = UnavailableSyncServerGateway();
    expect(stub.mode, 'stub');
    await expectLater(
      stub.requestChallenge(devId),
      throwsA(
        isA<ServerUnavailable>().having(
          (e) => e.retryAfter,
          'retryAfter',
          const Duration(minutes: 5),
        ),
      ),
    );
  });

  group('token', () {
    test('needsHandshake: nulo, a 6 min e a 4 min de expirar', () async {
      final now = DateTime.utc(2026, 10, 27, 12);
      expect(repo.needsHandshake(now, const Duration(minutes: 5)), isTrue);
      gateway.expiresAt = now.add(const Duration(minutes: 6));
      await repo.submitResponse(devId, response);
      expect(repo.token!.value, 'tok');
      expect(repo.needsHandshake(now, const Duration(minutes: 5)), isFalse);
      gateway.expiresAt = now.add(const Duration(minutes: 4));
      await repo.submitResponse(devId, response);
      expect(repo.needsHandshake(now, const Duration(minutes: 5)), isTrue);
      repo.clearToken();
      expect(repo.token, isNull);
    });

    test('suggestedLatency é repassada', () async {
      gateway.suggested = const Duration(seconds: 30);
      final out = await repo.submitResponse(devId, response);
      expect(out.suggestedLatency, const Duration(seconds: 30));
    });
  });

  group('desafio e resposta: exceção vira Failure', () {
    final cases = <(Exception, Matcher)>[
      (const ServerUnavailable(), isA<ChallengeUnavailable>()),
      (
        const ServerRateLimited(Duration(seconds: 3)),
        isA<RateLimitedFailure>(),
      ),
      (
        const ServerDongleBlocked('dongle_inativo'),
        isA<DongleBlockedFailure>(),
      ),
      (const ServerChallengeRejected(), isA<ChallengeRejected>()),
      (const ServerResponseRejected(), isA<ResponseRejected>()),
      (const ServerMalformed('tlv_invalido'), isA<ServerContractFailure>()),
    ];
    for (final (error, matcher) in cases) {
      test(error.runtimeType.toString(), () async {
        gateway.error = error;
        await expectLater(repo.requestChallenge(devId), throwsA(matcher));
        await expectLater(
          repo.submitResponse(devId, response),
          throwsA(matcher),
        );
      });
    }
  });

  group('lote: exceção vira PostResult', () {
    final blocks = [SyncFrame(SyncMessageType.block, Uint8List(0))];

    setUp(() async {
      await repo.submitResponse(devId, response);
    });

    test('sem token é Unauthorized', () async {
      repo.clearToken();
      expect(await repo.postBlocks(blocks), isA<Unauthorized>());
    });

    test('200 e lote_sem_progresso são BlocksAccepted', () async {
      expect(await repo.postBlocks(blocks), isA<BlocksAccepted>());
      gateway.error = ServerBatchNoProgress(_ack());
      expect(await repo.postBlocks(blocks), isA<BlocksAccepted>());
    });

    test('401 descarta o token', () async {
      gateway.error = const ServerUnauthorized();
      expect(await repo.postBlocks(blocks), isA<Unauthorized>());
      expect(repo.token, isNull);
    });

    test('403, bloqueio, 429, 5xx e 400', () async {
      gateway.error = const ServerForbiddenOtherDongle();
      expect(await repo.postBlocks(blocks), isA<Forbidden>());
      gateway.error = null;
      await repo.submitResponse(devId, response);
      gateway.error = const ServerDongleBlocked('dongle_desconhecido');
      expect(
        (await repo.postBlocks(blocks) as DongleBlocked).reason,
        'dongle_desconhecido',
      );
      gateway.error = const ServerRateLimited(Duration(seconds: 7));
      expect(
        (await repo.postBlocks(blocks) as RateLimited).retryAfter,
        const Duration(seconds: 7),
      );
      gateway.error = const ServerUnavailable(retryAfter: Duration(seconds: 2));
      expect(
        (await repo.postBlocks(blocks) as PostUnavailable).retryAfter,
        const Duration(seconds: 2),
      );
      gateway.error = const ServerMalformed('lote_malformado');
      expect(
        (await repo.postBlocks(blocks) as MalformedRequest).code,
        'lote_malformado',
      );
    });
  });

  group('datasource valida o contrato', () {
    test('desafio de tipo ou tamanho errado', () async {
      gateway.challenge = _challenge(44);
      await expectLater(
        repo.requestChallenge(devId),
        throwsA(isA<ServerContractFailure>()),
      );
      gateway.challenge = SyncFrame(SyncMessageType.ack, Uint8List(45));
      await expectLater(
        repo.requestChallenge(devId),
        throwsA(isA<ServerContractFailure>()),
      );
      gateway.challenge = _challenge(47);
      expect(await repo.requestChallenge(devId), isA<SyncFrame>());
    });

    test('ACK de tamanho errado na resposta e no lote', () async {
      gateway.ack = SyncFrame(SyncMessageType.ack, Uint8List(24));
      await expectLater(
        repo.submitResponse(devId, response),
        throwsA(isA<ServerContractFailure>()),
      );
      gateway.ack = _ack();
      await repo.submitResponse(devId, response);
      gateway.ack = SyncFrame(SyncMessageType.ack, Uint8List(24));
      final r = await repo.postBlocks([]);
      expect(r, isA<MalformedRequest>());
    });
  });
}
