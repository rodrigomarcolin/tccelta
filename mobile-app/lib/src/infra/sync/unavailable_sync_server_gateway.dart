import 'package:tccelta_mobile/src/domain/sync/dev_id.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_frame.dart';
import 'package:tccelta_mobile/src/services/sync/sync_server_exceptions.dart';
import 'package:tccelta_mobile/src/services/sync/sync_server_gateway.dart';

/// Servidor ausente: todas as chamadas falham como indisponível, e o app
/// segue em "handshake pendente" enquanto o dongle continua gravando.
class UnavailableSyncServerGateway implements SyncServerGateway {
  /// Cria o gateway.
  const UnavailableSyncServerGateway();

  static const ServerUnavailable _error = ServerUnavailable(
    retryAfter: Duration(minutes: 5),
  );

  @override
  String get mode => 'stub';

  @override
  Future<ChallengeEnvelope> requestChallenge(DevId devId) =>
      Future.error(_error);

  @override
  Future<HandshakeEnvelope> submitResponse(DevId devId, SyncFrame response) =>
      Future.error(_error);

  @override
  Future<SyncFrame> postBlocks(String token, List<SyncFrame> blocks) =>
      Future.error(_error);

  @override
  Future<SyncFrame?> latestConfirmation(DevId devId, String token) =>
      Future.error(_error);
}
