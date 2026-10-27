import 'package:tccelta_mobile/src/domain/sync/sync_frame.dart';

/// Erro do servidor de sincronização, já classificado pelo `codigo` (HTTP
/// 401/403/404/409/422/429/5xx). É o que o port levanta; o repository o
/// converte em resultado ou `Failure`.
abstract class SyncServerException implements Exception {
  /// Cria a exceção com a [message].
  const SyncServerException(this.message);

  /// Descrição legível.
  final String message;

  @override
  String toString() => '$runtimeType: $message';
}

/// 401: token ausente, inválido ou expirado.
class ServerUnauthorized extends SyncServerException {
  /// Cria a exceção; [code] é o `codigo` recebido.
  const ServerUnauthorized([this.code = 'token_invalido'])
    : super('Token recusado');

  /// `token_ausente`, `token_invalido` ou `token_expirado`.
  final String code;
}

/// 403: o token pertence a outro dongle.
class ServerForbiddenOtherDongle extends SyncServerException {
  /// Cria a exceção.
  const ServerForbiddenOtherDongle() : super('Token de outro dongle');
}

/// 403/404: dongle inativo ou desconhecido.
class ServerDongleBlocked extends SyncServerException {
  /// Cria a exceção com o [reason] (`dongle_inativo`, `dongle_desconhecido`).
  const ServerDongleBlocked(this.reason) : super('Dongle bloqueado');

  /// Motivo devolvido pelo servidor.
  final String reason;
}

/// 404: não há confirmação para o dongle.
class ServerNoConfirmation extends SyncServerException {
  /// Cria a exceção.
  const ServerNoConfirmation() : super('Sem confirmação');
}

/// 409: desafio inválido, reusado, expirado ou com `c_S` antigo.
class ServerChallengeRejected extends SyncServerException {
  /// Cria a exceção; [code] é o `codigo` recebido.
  const ServerChallengeRejected([this.code = 'desafio_invalido'])
    : super('Desafio recusado');

  /// `desafio_invalido` ou `c_s_antigo`.
  final String code;
}

/// 422: tag inválida ou resposta inconsistente.
class ServerResponseRejected extends SyncServerException {
  /// Cria a exceção; [inconsistent] distingue `resposta_inconsistente` de
  /// `tag_invalida`.
  const ServerResponseRejected({this.inconsistent = false})
    : super('Resposta recusada');

  /// `true` para `resposta_inconsistente`.
  final bool inconsistent;
}

/// 422 `lote_sem_progresso`: o lote não avançou; o corpo traz o ACK atual.
class ServerBatchNoProgress extends SyncServerException {
  /// Cria a exceção com o [ack] devolvido.
  const ServerBatchNoProgress(this.ack) : super('Lote sem progresso');

  /// ACK 0x04 devolvido junto.
  final SyncFrame ack;
}

/// 429: limite de taxa ou lote grande.
class ServerRateLimited extends SyncServerException {
  /// Cria a exceção com a espera exigida.
  const ServerRateLimited(this.retryAfter) : super('Limite de taxa');

  /// `Retry-After`.
  final Duration retryAfter;
}

/// 5xx, *timeout* ou sem rede.
class ServerUnavailable extends SyncServerException {
  /// Cria a exceção; [retryAfter] é o `Retry-After`, se houver.
  const ServerUnavailable({this.retryAfter}) : super('Servidor indisponível');

  /// Espera sugerida pelo servidor.
  final Duration? retryAfter;
}

/// 400/413/415: o servidor não aceitou a forma da requisição, ou devolveu algo
/// fora do contrato.
class ServerMalformed extends SyncServerException {
  /// Cria a exceção com o [code] recebido.
  const ServerMalformed(this.code) : super('Requisição malformada');

  /// `corpo_malformado`, `tlv_invalido`, `lote_malformado`, etc.
  final String code;
}
