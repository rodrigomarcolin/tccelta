import 'dart:async';
import 'dart:collection';

import 'package:tccelta_mobile/src/core/errors/sync_failure.dart';
import 'package:tccelta_mobile/src/data/datasources/sync_server_datasource.dart';
import 'package:tccelta_mobile/src/data/repositories/sync_server_repository_impl.dart';
import 'package:tccelta_mobile/src/domain/repositories/sync_server_repository.dart';
import 'package:tccelta_mobile/src/domain/sync/dev_id.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_frame.dart';
import 'package:tccelta_mobile/src/infra/sync/bench/bench_sync_server.dart';
import 'package:tccelta_mobile/src/infra/sync/bench/bench_sync_server_gateway.dart';

/// [SyncServerRepository] roteirizável sobre o servidor de bancada: delega a
/// um [SyncServerRepositoryImpl] real, mas deixa o teste injetar resultados de
/// POST, falhas de desafio e comportas (`Completer`) em chamadas em voo.
class FakeSyncServerRepository implements SyncServerRepository {
  /// Cria o dublê (com um [BenchSyncServer] novo se [bench] for nulo).
  FakeSyncServerRepository({BenchSyncServer? bench})
    : bench = bench ?? BenchSyncServer() {
    _inner = SyncServerRepositoryImpl(
      SyncServerDatasource(BenchSyncServerGateway(this.bench)),
    );
  }

  /// Servidor de bancada por baixo.
  final BenchSyncServer bench;
  late final SyncServerRepositoryImpl _inner;

  /// Blocos de cada POST recebido, na ordem.
  final List<List<SyncFrame>> posted = [];

  /// Registro de chamadas (`challenge`, `response`, `post`), na ordem.
  final List<String> log = [];

  /// Resultados de POST consumidos antes de delegar ao servidor.
  final Queue<PostResult> scripted = Queue<PostResult>();

  /// Se não nulo, `requestChallenge` falha com isto.
  SyncFailure? challengeFailure;

  /// Força `needsHandshake` até o próximo `submitResponse`.
  bool forceNeedsHandshake = false;

  /// Se não nulo, o POST espera este *completer* antes de delegar.
  Completer<void>? postGate;

  /// Se não nulo, `submitResponse` espera este *completer*.
  Completer<void>? submitGate;

  /// `T_lat` sugerido devolvido no handshake.
  Duration? suggestedLatency;

  /// Handshakes concluídos.
  int handshakes = 0;

  @override
  String get mode => 'fake';

  @override
  SyncToken? get token => _inner.token;

  @override
  bool needsHandshake(DateTime now, Duration margin) =>
      forceNeedsHandshake || _inner.needsHandshake(now, margin);

  @override
  void clearToken() => _inner.clearToken();

  @override
  Future<SyncFrame> requestChallenge(DevId devId) async {
    log.add('challenge');
    final failure = challengeFailure;
    if (failure != null) throw failure;
    return _inner.requestChallenge(devId);
  }

  @override
  Future<HandshakeOutcome> submitResponse(
    DevId devId,
    SyncFrame response,
  ) async {
    log.add('response');
    await submitGate?.future;
    final out = await _inner.submitResponse(devId, response);
    forceNeedsHandshake = false;
    handshakes++;
    return HandshakeOutcome(
      ack: out.ack,
      token: out.token,
      suggestedLatency: suggestedLatency,
    );
  }

  @override
  Future<PostResult> postBlocks(List<SyncFrame> blocks) async {
    log.add('post');
    posted.add(blocks);
    await postGate?.future;
    if (scripted.isNotEmpty) {
      final r = scripted.removeFirst();
      if (r is Unauthorized) _inner.clearToken();
      return r;
    }
    return _inner.postBlocks(blocks);
  }
}
