import 'package:tccelta_mobile/src/core/errors/sync_failure.dart';
import 'package:tccelta_mobile/src/data/datasources/sync_server_datasource.dart';
import 'package:tccelta_mobile/src/domain/repositories/sync_server_repository.dart';
import 'package:tccelta_mobile/src/domain/sync/dev_id.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_frame.dart';
import 'package:tccelta_mobile/src/services/sync/sync_server_exceptions.dart';

/// *Source of truth* do token e do resultado das chamadas ao servidor.
/// Converte as exceções do port em `Failure` (desafio/resposta) ou em
/// [PostResult] (lotes). O token vive só em memória.
class SyncServerRepositoryImpl implements SyncServerRepository {
  /// Cria o repository sobre o [_ds].
  SyncServerRepositoryImpl(this._ds);

  final SyncServerDatasource _ds;
  SyncToken? _token;

  @override
  String get mode => _ds.mode;

  @override
  SyncToken? get token => _token;

  @override
  bool needsHandshake(DateTime now, Duration margin) {
    final t = _token;
    return t == null || t.expiresAt.difference(now) < margin;
  }

  @override
  void clearToken() => _token = null;

  @override
  Future<SyncFrame> requestChallenge(DevId devId) async {
    try {
      return (await _ds.requestChallenge(devId)).challenge;
    } on SyncServerException catch (e) {
      throw _failure(e);
    }
  }

  @override
  Future<HandshakeOutcome> submitResponse(
    DevId devId,
    SyncFrame response,
  ) async {
    try {
      final env = await _ds.submitResponse(devId, response);
      final token = SyncToken(
        value: env.token,
        expiresAt: env.tokenExpiresAt.toUtc(),
      );
      _token = token;
      return HandshakeOutcome(
        ack: env.ack,
        token: token,
        suggestedLatency: env.suggestedLatency,
      );
    } on SyncServerException catch (e) {
      throw _failure(e);
    }
  }

  @override
  Future<PostResult> postBlocks(List<SyncFrame> blocks) async {
    final t = _token;
    if (t == null) return const Unauthorized();
    try {
      return BlocksAccepted(await _ds.postBlocks(t.value, blocks));
    } on ServerBatchNoProgress catch (e) {
      return BlocksAccepted(e.ack);
    } on ServerUnauthorized {
      _token = null;
      return const Unauthorized();
    } on ServerForbiddenOtherDongle {
      _token = null;
      return const Forbidden();
    } on ServerDongleBlocked catch (e) {
      return DongleBlocked(e.reason);
    } on ServerRateLimited catch (e) {
      return RateLimited(e.retryAfter);
    } on ServerUnavailable catch (e) {
      return PostUnavailable(e.retryAfter);
    } on ServerMalformed catch (e) {
      return MalformedRequest(e.code);
    } on SyncServerException catch (e) {
      return MalformedRequest(e.message);
    }
  }

  SyncFailure _failure(SyncServerException e) => switch (e) {
    ServerUnavailable(:final retryAfter) => ChallengeUnavailable(
      'Servidor indisponível',
      retryAfter: retryAfter,
    ),
    ServerRateLimited(:final retryAfter) => RateLimitedFailure(
      'Limite de taxa do servidor',
      retryAfter: retryAfter,
    ),
    ServerDongleBlocked(:final reason) => DongleBlockedFailure(
      'Dongle bloqueado no servidor',
      reason: reason,
    ),
    ServerChallengeRejected() => const ChallengeRejected('Desafio recusado'),
    ServerResponseRejected() => const ResponseRejected('Resposta recusada'),
    ServerMalformed(:final code) => ServerContractFailure(
      'Fora do contrato',
      code: code,
    ),
    _ => ServerContractFailure(e.message),
  };
}
