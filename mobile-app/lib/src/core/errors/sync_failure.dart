import 'package:tccelta_mobile/src/core/errors/failure.dart';

/// Falha da sincronização com o dongle ou com o servidor.
abstract class SyncFailure extends Failure {
  /// Cria a falha com [message].
  const SyncFailure(super.message);
}

/// Falha de escrita ou indisponibilidade do canal SYNC do dongle.
class SyncLinkFailure extends SyncFailure {
  /// Cria a falha com [message] e a [cause] crua opcional.
  const SyncLinkFailure(super.message, {this.cause});

  /// Erro cru que originou a falha.
  final Object? cause;
}

/// Servidor sem resposta ao pedir o desafio (rede, 5xx, não integrado).
class ChallengeUnavailable extends SyncFailure {
  /// Cria a falha; [retryAfter] sugere quando tentar de novo.
  const ChallengeUnavailable(super.message, {this.retryAfter});

  /// Espera sugerida pelo servidor.
  final Duration? retryAfter;
}

/// Servidor recusou o desafio (409 ou desafio inválido).
class ChallengeRejected extends SyncFailure {
  /// Cria a falha.
  const ChallengeRejected(super.message);
}

/// Servidor recusou a resposta do dongle (tag ou conteúdo inválidos).
class ResponseRejected extends SyncFailure {
  /// Cria a falha.
  const ResponseRejected(super.message);
}

/// Dongle inativo ou desconhecido no servidor.
class DongleBlockedFailure extends SyncFailure {
  /// Cria a falha com o [reason] devolvido pelo servidor.
  const DongleBlockedFailure(super.message, {required this.reason});

  /// Motivo (`dongle_inativo`, `dongle_desconhecido`).
  final String reason;
}

/// Servidor limitou a taxa (429).
class RateLimitedFailure extends SyncFailure {
  /// Cria a falha com [retryAfter].
  const RateLimitedFailure(super.message, {required this.retryAfter});

  /// Espera exigida pelo servidor.
  final Duration retryAfter;
}

/// Servidor indisponível (5xx, rede, timeout).
class ServerUnavailableFailure extends SyncFailure {
  /// Cria a falha; [retryAfter] sugere quando tentar de novo.
  const ServerUnavailableFailure(super.message, {this.retryAfter});

  /// Espera sugerida pelo servidor.
  final Duration? retryAfter;
}

/// Servidor devolveu algo fora do contrato (ou 400/413/415).
class ServerContractFailure extends SyncFailure {
  /// Cria a falha com o [code] devolvido, se houver.
  const ServerContractFailure(super.message, {this.code});

  /// Código do problema (`codigo`).
  final String? code;
}

/// O dongle respondeu ao desafio com erro 0x01 ou 0x02.
class HandshakeRejectedByDongle extends SyncFailure {
  /// Cria a falha com o [code] do erro.
  const HandshakeRejectedByDongle(this.code)
    : super('Dongle recusou o desafio');

  /// Byte do erro (Tabela 5).
  final int code;
}

/// O dongle recusou o ACK de abertura com erro 0x01 ou 0x03.
class HandshakeAckRejected extends SyncFailure {
  /// Cria a falha com o [code] do erro.
  const HandshakeAckRejected(this.code) : super('Dongle recusou o ACK');

  /// Byte do erro (Tabela 5).
  final int code;
}

/// O link SYNC caiu (ou mudou de geração) durante o handshake.
class HandshakeLinkLost extends SyncFailure {
  /// Cria a falha.
  const HandshakeLinkLost() : super('Link SYNC perdido durante o handshake');
}

/// O dongle não respondeu a tempo ao desafio.
class HandshakeTimeout extends SyncFailure {
  /// Cria a falha.
  const HandshakeTimeout() : super('Dongle não respondeu ao desafio');
}
