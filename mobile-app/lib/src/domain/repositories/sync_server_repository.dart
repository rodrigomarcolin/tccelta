import 'package:meta/meta.dart';
import 'package:tccelta_mobile/src/domain/sync/dev_id.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_frame.dart';

/// Token de acesso do servidor; só em memória.
@immutable
class SyncToken {
  /// Cria o token.
  const SyncToken({required this.value, required this.expiresAt});

  /// Valor opaco (JWT).
  final String value;

  /// Validade (UTC).
  final DateTime expiresAt;
}

/// Resultado de um handshake bem-sucedido.
@immutable
class HandshakeOutcome {
  /// Cria o resultado.
  const HandshakeOutcome({
    required this.ack,
    required this.token,
    this.suggestedLatency,
  });

  /// ACK 0x04 a escrever no dongle.
  final SyncFrame ack;

  /// Token guardado no repository.
  final SyncToken token;

  /// `T_lat` sugerido pelo servidor.
  final Duration? suggestedLatency;
}

/// Resultado do envio de um lote.
sealed class PostResult {
  const PostResult();
}

/// Lote aceito (200) ou sem progresso (422 `lote_sem_progresso`).
final class BlocksAccepted extends PostResult {
  /// Cria o resultado com o [ack].
  const BlocksAccepted(this.ack);

  /// ACK 0x04 devolvido.
  final SyncFrame ack;
}

/// 401: token recusado.
final class Unauthorized extends PostResult {
  /// Cria o resultado.
  const Unauthorized();
}

/// 403: token de outro dongle.
final class Forbidden extends PostResult {
  /// Cria o resultado.
  const Forbidden();
}

/// 403/404: dongle inativo ou desconhecido.
final class DongleBlocked extends PostResult {
  /// Cria o resultado com o [reason].
  const DongleBlocked(this.reason);

  /// Motivo devolvido.
  final String reason;
}

/// 429: limite de taxa.
final class RateLimited extends PostResult {
  /// Cria o resultado.
  const RateLimited(this.retryAfter);

  /// Espera exigida.
  final Duration retryAfter;
}

/// 5xx, rede ou *timeout*.
final class PostUnavailable extends PostResult {
  /// Cria o resultado.
  const PostUnavailable(this.retryAfter);

  /// Espera sugerida pelo servidor.
  final Duration? retryAfter;
}

/// 400/413/415: requisição malformada.
final class MalformedRequest extends PostResult {
  /// Cria o resultado com o [code].
  const MalformedRequest(this.code);

  /// Código do problema.
  final String code;
}

/// Servidor de sincronização: token, handshake e envio de lotes. Erros do
/// desafio e da resposta saem como `SyncFailure`; o envio de lotes devolve um
/// [PostResult].
abstract interface class SyncServerRepository {
  /// Modo do servidor (`stub`, `bench`, `https`).
  String get mode;

  /// Token guardado, se houver.
  SyncToken? get token;

  /// `true` se não há token ou ele expira em menos de [margin] a partir de
  /// [now].
  bool needsHandshake(DateTime now, Duration margin);

  /// Pede o Desafio 0x01.
  Future<SyncFrame> requestChallenge(DevId devId);

  /// Envia a Resposta 0x02 e guarda o token.
  Future<HandshakeOutcome> submitResponse(DevId devId, SyncFrame response);

  /// Envia os blocos usando o token guardado.
  Future<PostResult> postBlocks(List<SyncFrame> blocks);

  /// Esquece o token.
  void clearToken();
}
