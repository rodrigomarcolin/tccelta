import 'package:meta/meta.dart';
import 'package:tccelta_mobile/src/domain/sync/dev_id.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_frame.dart';

/// Servidor de sincronização visto pelo app. Transporta mensagens TLV
/// ([SyncFrame]) sem interpretá-las: o app nunca altera os bytes. Falhas saem
/// como `SyncServerException` (ver `sync_server_exceptions.dart`).
abstract interface class SyncServerGateway {
  /// Modo exibido na UI: `stub`, `bench` ou `https`.
  String get mode;

  /// Pede o Desafio 0x01 (V com 45/47 B) para o [devId].
  Future<ChallengeEnvelope> requestChallenge(DevId devId);

  /// Envia a Resposta 0x02 do dongle e recebe o ACK 0x04 e o token.
  Future<HandshakeEnvelope> submitResponse(DevId devId, SyncFrame response);

  /// Envia 1–32 blocos 0x03, na ordem, e recebe o ACK 0x04.
  Future<SyncFrame> postBlocks(String token, List<SyncFrame> blocks);

  /// Última confirmação do servidor (opcional; `null` = sem confirmação).
  Future<SyncFrame?> latestConfirmation(DevId devId, String token);
}

/// Desafio devolvido pelo servidor.
@immutable
class ChallengeEnvelope {
  /// Cria o envelope.
  const ChallengeEnvelope({required this.challenge, this.expiresAt});

  /// Frame tipo 0x01.
  final SyncFrame challenge;

  /// Validade do desafio, se informada.
  final DateTime? expiresAt;
}

/// Resultado do handshake devolvido pelo servidor.
@immutable
class HandshakeEnvelope {
  /// Cria o envelope.
  const HandshakeEnvelope({
    required this.ack,
    required this.token,
    required this.tokenExpiresAt,
    this.suggestedLatency,
  });

  /// Frame tipo 0x04, a escrever no dongle.
  final SyncFrame ack;

  /// Token de acesso (opaco para o app).
  final String token;

  /// Validade do token (UTC).
  final DateTime tokenExpiresAt;

  /// `T_lat` sugerido pelo servidor, se houver.
  final Duration? suggestedLatency;
}
