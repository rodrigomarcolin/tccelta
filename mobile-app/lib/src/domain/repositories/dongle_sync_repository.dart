import 'package:tccelta_mobile/src/domain/sync/fragment_assembler.dart';
import 'package:tccelta_mobile/src/domain/sync/handshake_messages.dart';
import 'package:tccelta_mobile/src/domain/sync/new_block_notice.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_block.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_frame.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_protocol.dart';

/// Evento vindo do dongle pelo serviço SYNC.
sealed class DongleSyncEvent {
  const DongleSyncEvent();
}

/// O link SYNC subiu ou caiu; [generation] identifica o link.
final class SyncLinkChanged extends DongleSyncEvent {
  /// Cria o evento.
  const SyncLinkChanged({required this.up, required this.generation});

  /// `true` quando o link ficou utilizável.
  final bool up;

  /// Geração do link (incrementa a cada subida).
  final int generation;
}

/// Aviso de novo bloco.
final class NoticeReceived extends DongleSyncEvent {
  /// Cria o evento.
  const NoticeReceived(this.notice);

  /// Aviso recebido.
  final NewBlockNotice notice;
}

/// Bloco lido do dongle.
final class BlockReceived extends DongleSyncEvent {
  /// Cria o evento.
  const BlockReceived(this.block);

  /// Bloco recebido.
  final SyncBlock block;
}

/// Resposta 0x02 ao desafio.
final class ResponseReceived extends DongleSyncEvent {
  /// Cria o evento.
  const ResponseReceived(this.frame, this.info);

  /// Frame original, a repassar ao servidor sem alteração.
  final SyncFrame frame;

  /// Campos públicos da resposta.
  final ResponseInfo info;
}

/// Erro enviado pelo dongle.
final class DongleErrorReceived extends DongleSyncEvent {
  /// Cria o evento.
  const DongleErrorReceived(this.code);

  /// Código do erro.
  final SyncErrorCode code;
}

/// Uma mensagem do dongle chegou incompleta ou fora de ordem.
final class TransportIncomplete extends DongleSyncEvent {
  /// Cria o evento.
  const TransportIncomplete(this.kind);

  /// Motivo da falha de montagem.
  final AssemblyErrorKind kind;
}

/// Canal SYNC com o dongle: mensagens tipadas de entrada e saída.
abstract interface class DongleSyncRepository {
  /// Eventos do dongle (broadcast).
  Stream<DongleSyncEvent> get events;

  /// `true` com o link SYNC utilizável.
  bool get isLinkUp;

  /// MTU ATT da conexão atual (23 se desconhecido).
  int get mtu;

  /// Escreve o Desafio 0x01 (valida tipo e `L` 45/47).
  Future<void> sendChallenge(SyncFrame challenge);

  /// Escreve o ACK 0x04 (valida tipo e `L` 25).
  Future<void> sendAck(SyncFrame ack);

  /// Pede um lote de [k] blocos (1…65535).
  Future<void> requestBatch(int k);

  /// Envia um erro ao dongle (só 0x04 e 0x07).
  Future<void> sendError(SyncErrorCode code);
}
