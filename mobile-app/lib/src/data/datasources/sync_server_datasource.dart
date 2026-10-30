import 'package:tccelta_mobile/src/domain/sync/dev_id.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_frame.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_protocol.dart';
import 'package:tccelta_mobile/src/services/sync/sync_server_exceptions.dart';
import 'package:tccelta_mobile/src/services/sync/sync_server_gateway.dart';

/// Envolve o [SyncServerGateway] e valida o formato do que volta: desafio
/// tipo 0x01 com `L` 45/47 e ACK tipo 0x04 com `L` 25.
class SyncServerDatasource {
  /// Cria o datasource sobre o [_gateway].
  SyncServerDatasource(this._gateway);

  final SyncServerGateway _gateway;

  /// Modo do servidor.
  String get mode => _gateway.mode;

  /// Pede o desafio e confere o formato.
  Future<ChallengeEnvelope> requestChallenge(DevId devId) async {
    final envelope = await _gateway.requestChallenge(devId);
    final c = envelope.challenge;
    final n = c.value.length;
    if (c.type != SyncMessageType.challenge ||
        (n != kChallengeLength && n != kChallengeDiagLength)) {
      throw const ServerMalformed('desafio_fora_do_contrato');
    }
    return envelope;
  }

  /// Envia a resposta e confere o ACK devolvido.
  Future<HandshakeEnvelope> submitResponse(
    DevId devId,
    SyncFrame response,
  ) async {
    final envelope = await _gateway.submitResponse(devId, response);
    _checkAck(envelope.ack);
    return envelope;
  }

  /// Envia o lote e confere o ACK devolvido (inclusive o de
  /// `ServerBatchNoProgress`).
  Future<SyncFrame> postBlocks(String token, List<SyncFrame> blocks) async {
    try {
      final ack = await _gateway.postBlocks(token, blocks);
      _checkAck(ack);
      return ack;
    } on ServerBatchNoProgress catch (e) {
      _checkAck(e.ack);
      rethrow;
    }
  }

  /// Última confirmação, se houver.
  Future<SyncFrame?> latestConfirmation(DevId devId, String token) async {
    final ack = await _gateway.latestConfirmation(devId, token);
    if (ack != null) _checkAck(ack);
    return ack;
  }

  void _checkAck(SyncFrame ack) {
    if (ack.type != SyncMessageType.ack || ack.value.length != kAckLength) {
      throw const ServerMalformed('ack_fora_do_contrato');
    }
  }
}
