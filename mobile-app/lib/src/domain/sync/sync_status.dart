import 'package:meta/meta.dart';
import 'package:tccelta_mobile/src/domain/sync/dev_id.dart';
import 'package:tccelta_mobile/src/domain/sync/new_block_notice.dart';

/// Fase da sincronização vista pelo usuário.
enum SyncPhase {
  /// Sem link SYNC com o dongle.
  offline,

  /// Link de pé, aguardando o aviso do dongle.
  waitingNotice,

  /// Handshake em andamento.
  handshaking,

  /// Handshake falhou; aguardando a próxima tentativa.
  handshakePending,

  /// Sessão aberta; leitura contínua.
  open,

  /// Ressincronizando a partir do ACK.
  resyncing,

  /// Parada (dongle bloqueado no servidor).
  stopped,
}

/// Origem do `T_lat` efetivo.
enum LatencySource {
  /// Configuração local.
  local,

  /// Teto sugerido pelo servidor.
  server,
}

/// Estado observável da sincronização.
@immutable
class SyncStatus {
  /// Cria o estado.
  const SyncStatus({
    this.phase = SyncPhase.offline,
    this.devId,
    this.headSeq,
    this.appAck,
    this.lastRead,
    this.buffered = 0,
    this.bufferCapacity = 0,
    this.inFlight = 0,
    this.flags,
    this.nextAttemptAt,
    this.lastError,
    this.serverMode = '',
    this.postsOk = 0,
    this.postsFailed = 0,
    this.effectiveLatency = const Duration(seconds: 60),
    this.latencySource = LatencySource.local,
  });

  /// Fase atual.
  final SyncPhase phase;

  /// Identidade do dongle, quando já avisada.
  final DevId? devId;

  /// Último `seq` gravado no dongle.
  final int? headSeq;

  /// Último `seq` confirmado pelo servidor.
  final int? appAck;

  /// Último `seq` lido (espelho do cursor).
  final int? lastRead;

  /// Blocos no buffer.
  final int buffered;

  /// Capacidade do buffer (`B_max`).
  final int bufferCapacity;

  /// Blocos em POST.
  final int inFlight;

  /// Flags do último aviso.
  final NoticeFlags? flags;

  /// Próxima tentativa, em tempo de parede (UTC), se houver espera.
  final DateTime? nextAttemptAt;

  /// Última falha legível.
  final String? lastError;

  /// Modo do servidor (`stub`, `bench`, `https`).
  final String serverMode;

  /// POSTs aceitos.
  final int postsOk;

  /// POSTs com falha.
  final int postsFailed;

  /// `T_lat` efetivo.
  final Duration effectiveLatency;

  /// Origem do `T_lat` efetivo.
  final LatencySource latencySource;

  static const Object _keep = Object();

  /// Cópia com campos trocados; passe `null` explícito para limpar os
  /// anuláveis.
  SyncStatus copyWith({
    SyncPhase? phase,
    Object? devId = _keep,
    Object? headSeq = _keep,
    Object? appAck = _keep,
    Object? lastRead = _keep,
    int? buffered,
    int? bufferCapacity,
    int? inFlight,
    Object? flags = _keep,
    Object? nextAttemptAt = _keep,
    Object? lastError = _keep,
    String? serverMode,
    int? postsOk,
    int? postsFailed,
    Duration? effectiveLatency,
    LatencySource? latencySource,
  }) => SyncStatus(
    phase: phase ?? this.phase,
    devId: identical(devId, _keep) ? this.devId : devId as DevId?,
    headSeq: identical(headSeq, _keep) ? this.headSeq : headSeq as int?,
    appAck: identical(appAck, _keep) ? this.appAck : appAck as int?,
    lastRead: identical(lastRead, _keep) ? this.lastRead : lastRead as int?,
    buffered: buffered ?? this.buffered,
    bufferCapacity: bufferCapacity ?? this.bufferCapacity,
    inFlight: inFlight ?? this.inFlight,
    flags: identical(flags, _keep) ? this.flags : flags as NoticeFlags?,
    nextAttemptAt: identical(nextAttemptAt, _keep)
        ? this.nextAttemptAt
        : nextAttemptAt as DateTime?,
    lastError: identical(lastError, _keep)
        ? this.lastError
        : lastError as String?,
    serverMode: serverMode ?? this.serverMode,
    postsOk: postsOk ?? this.postsOk,
    postsFailed: postsFailed ?? this.postsFailed,
    effectiveLatency: effectiveLatency ?? this.effectiveLatency,
    latencySource: latencySource ?? this.latencySource,
  );
}
