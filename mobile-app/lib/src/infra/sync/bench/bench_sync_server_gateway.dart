import 'package:tccelta_mobile/src/domain/sync/dev_id.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_frame.dart';
import 'package:tccelta_mobile/src/infra/sync/bench/bench_faults.dart';
import 'package:tccelta_mobile/src/infra/sync/bench/bench_sync_server.dart';
import 'package:tccelta_mobile/src/services/sync/sync_server_exceptions.dart';
import 'package:tccelta_mobile/src/services/sync/sync_server_gateway.dart';

/// Gateway que envolve o [BenchSyncServer], aplicando o atraso e as falhas
/// injetadas.
class BenchSyncServerGateway implements SyncServerGateway {
  /// Cria o gateway sobre o [server].
  BenchSyncServerGateway(this.server, {this.faults = const BenchFaults()});

  /// Servidor de bancada.
  final BenchSyncServer server;

  /// Falhas injetadas.
  final BenchFaults faults;

  int _calls = 0;

  @override
  String get mode => 'bench';

  Future<void> _gate() async {
    _calls++;
    if (faults.delay > Duration.zero) await Future<void>.delayed(faults.delay);
    bool hits(int every) => every > 0 && _calls % every == 0;
    if (hits(faults.fail503Every)) {
      throw const ServerUnavailable(retryAfter: Duration(seconds: 1));
    }
    if (hits(faults.fail401Every)) throw const ServerUnauthorized();
    if (hits(faults.fail429Every)) {
      throw const ServerRateLimited(Duration(seconds: 1));
    }
  }

  @override
  Future<ChallengeEnvelope> requestChallenge(DevId devId) async {
    await _gate();
    return server.requestChallenge(devId);
  }

  @override
  Future<HandshakeEnvelope> submitResponse(
    DevId devId,
    SyncFrame response,
  ) async {
    await _gate();
    return server.submitResponse(devId, response);
  }

  @override
  Future<SyncFrame> postBlocks(String token, List<SyncFrame> blocks) async {
    await _gate();
    return server.postBlocks(token, blocks);
  }

  @override
  Future<SyncFrame?> latestConfirmation(DevId devId, String token) async =>
      null;
}
