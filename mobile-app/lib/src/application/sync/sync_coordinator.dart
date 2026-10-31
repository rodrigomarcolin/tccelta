import 'dart:async';
import 'dart:math';

import 'package:flutter/foundation.dart';
import 'package:tccelta_mobile/src/application/sync/handshake_runner.dart';
import 'package:tccelta_mobile/src/core/errors/sync_failure.dart';
import 'package:tccelta_mobile/src/core/time/monotonic_clock.dart';
import 'package:tccelta_mobile/src/domain/repositories/dongle_sync_repository.dart';
import 'package:tccelta_mobile/src/domain/repositories/sync_server_repository.dart';
import 'package:tccelta_mobile/src/domain/repositories/sync_settings_repository.dart';
import 'package:tccelta_mobile/src/domain/sync/dev_id.dart';
import 'package:tccelta_mobile/src/domain/sync/handshake_messages.dart';
import 'package:tccelta_mobile/src/domain/sync/new_block_notice.dart';
import 'package:tccelta_mobile/src/domain/sync/retry_backoff.dart';
import 'package:tccelta_mobile/src/domain/sync/send_policy.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_block.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_buffer.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_frame.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_protocol.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_settings.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_status.dart';

enum _Session { none, handshaking, open }

/// Orquestra a leitura contínua (Fig. 15) e o aviso de novo bloco (Fig. 14):
/// pede lotes ao dongle, guarda os blocos no buffer, decide quando postar ao
/// servidor, repassa o ACK ao dongle e trata os erros da Tabela 5. Combina o
/// canal do dongle, o servidor e as configurações.
class SyncCoordinator {
  /// Cria o coordenador.
  SyncCoordinator({
    required this.dongle,
    required this.server,
    required SyncSettingsRepository settings,
    required this.handshake,
    MonotonicClock? clock,
    this.tickInterval = const Duration(seconds: 1),
    DateTime Function()? wallClock,
    RetryBackoff? backoff,
  }) : _settings = settings,
       _clock = clock ?? SystemMonotonicClock(),
       _wall = wallClock ?? DateTime.now,
       _backoff = backoff ?? RetryBackoff() {
    _buffer = SyncBuffer(capacity: settings.current.bufferCapacity);
    _status = _build();
  }

  /// Período do *tick* que reavalia o envio.
  final Duration tickInterval;

  /// Canal SYNC com o dongle.
  final DongleSyncRepository dongle;

  /// Servidor de sincronização.
  final SyncServerRepository server;

  /// Executor da perna BLE do handshake.
  final HandshakeRunner handshake;

  final SyncSettingsRepository _settings;
  final MonotonicClock _clock;
  final DateTime Function() _wall;
  final RetryBackoff _backoff;

  late final SyncBuffer _buffer;
  final StreamController<SyncStatus> _statusCtrl =
      StreamController<SyncStatus>.broadcast();
  late SyncStatus _status;

  StreamSubscription<DongleSyncEvent>? _eventsSub;
  StreamSubscription<SyncSettings>? _settingsSub;
  Timer? _timer;
  bool _disposed = false;

  bool _linkUp = false;
  int _generation = 0;
  DevId? _devId;
  int? _headSeq;
  NoticeFlags? _flags;
  _Session _session = _Session.none;
  bool _resyncing = false;
  int? _lastRead;
  int? _appAck;
  bool _awaitingBatch = false;
  int _batchTarget = 0;
  Duration _lastBlockAt = Duration.zero;
  bool _postInFlight = false;
  int _lotLimit = kMaxBlocksPerPost;
  Duration? _notBefore;
  DateTime? _nextAttemptAt;
  SyncFrame? _openingAck;
  bool _rewroteOpeningAck = false;
  SyncFrame? _heldAck;
  int? _interruptedAck;
  int _interruptedCount = 0;
  bool _stopped = false;
  bool _draining = false;
  Duration? _suggested;
  String? _lastError;
  int _postsOk = 0;
  int _postsFailed = 0;
  Future<bool>? _handshakeFuture;
  HandshakeTrigger _lastTrigger = HandshakeTrigger.noticeNoSession;

  /// Estado observável (broadcast).
  Stream<SyncStatus> get status => _statusCtrl.stream;

  /// Estado atual.
  SyncStatus get current => _status;

  /// Começa a escutar o dongle, as configurações e o relógio.
  void start() {
    if (_eventsSub != null) return;
    _eventsSub = dongle.events.listen(_onEvent);
    _settingsSub = _settings.watch().listen((_) {
      _buffer.capacity = _settings.current.bufferCapacity;
      _emit();
      _evaluateSend();
      _maybeRequestBatch();
    });
    _timer = Timer.periodic(tickInterval, (_) => _tick());
    if (dongle.isLinkUp) {
      _generation = dongle.linkGeneration;
      _linkUp = true;
      _status = _build(phase: SyncPhase.waitingNotice);
      _emitRaw();
    }
  }

  /// Encerra o coordenador.
  Future<void> dispose() async {
    _disposed = true;
    _timer?.cancel();
    await _eventsSub?.cancel();
    await _settingsSub?.cancel();
    await _statusCtrl.close();
  }

  /// Ação de depuração: refaz o handshake agora.
  Future<void> forceHandshake() async {
    _stopped = false;
    _notBefore = null;
    _backoff.reset();
    await _ensureHandshake(HandshakeTrigger.manual);
  }

  // estado

  Duration get _effectiveLatency {
    final local = _settings.current.latency;
    final s = _suggested;
    return s != null && s < local ? s : local;
  }

  SyncPhase get _phase {
    if (_stopped) return SyncPhase.stopped;
    if (!_linkUp) return SyncPhase.offline;
    if (_resyncing) return SyncPhase.resyncing;
    return switch (_session) {
      _Session.open => SyncPhase.open,
      _Session.handshaking => SyncPhase.handshaking,
      _Session.none =>
        _notBefore != null || _lastError != null
            ? SyncPhase.handshakePending
            : SyncPhase.waitingNotice,
    };
  }

  SyncStatus _build({SyncPhase? phase}) => SyncStatus(
    phase: phase ?? _phase,
    devId: _devId,
    headSeq: _headSeq,
    appAck: _appAck,
    lastRead: _lastRead,
    buffered: _buffer.length,
    bufferCapacity: _buffer.capacity,
    inFlight: _buffer.inFlightCount,
    flags: _flags,
    nextAttemptAt: _nextAttemptAt,
    lastError: _lastError,
    serverMode: server.mode,
    postsOk: _postsOk,
    postsFailed: _postsFailed,
    effectiveLatency: _effectiveLatency,
    latencySource: _suggested != null && _suggested! < _settings.current.latency
        ? LatencySource.server
        : LatencySource.local,
  );

  void _emit() {
    _status = _build();
    _emitRaw();
  }

  void _emitRaw() {
    if (!_statusCtrl.isClosed) _statusCtrl.add(_status);
  }

  void _log(String event, [String kv = '']) {
    if (kDebugMode) {
      debugPrint('SYNC ${_clock.now().inMilliseconds} $event $kv'.trimRight());
    }
  }

  // eventos

  void _onEvent(DongleSyncEvent e) {
    if (_disposed) return;
    switch (e) {
      case SyncLinkChanged():
        _onLink(e);
      case NoticeReceived():
        _onNotice(e.notice);
      case BlockReceived():
        _onBlock(e.block);
      case ResponseReceived():
        break; // consumida pelo HandshakeRunner
      case DongleErrorReceived():
        _onDongleError(e.code);
      case TransportIncomplete():
        _log('TRANSPORT_INCOMPLETE', 'kind=${e.kind.name}');
        if (_session == _Session.open) _resync(SyncErrorCode.incompleteMessage);
    }
    _emit();
  }

  void _onLink(SyncLinkChanged e) {
    _generation = e.generation;
    _log('LINK', 'up=${e.up ? 1 : 0} gen=${e.generation}');
    if (e.up) {
      _linkUp = true;
      _session = _Session.none;
      _resyncing = false;
      _awaitingBatch = false;
      _openingAck = null;
      _rewroteOpeningAck = false;
      _interruptedAck = null;
      _interruptedCount = 0;
      _stopped = false;
      _buffer.dropPending();
      _lastRead = _appAck;
    } else {
      _linkUp = false;
      _session = _Session.none;
      _awaitingBatch = false;
      _resyncing = false;
    }
  }

  void _onNotice(NewBlockNotice n) {
    _devId = n.devId;
    _headSeq = n.headSeq;
    _flags = n.flags;
    _log(
      'NOTICE',
      'head=${n.headSeq} flags=${n.flags.raw.toRadixString(16)} '
          'last=$_lastRead',
    );
    if (_stopped) return;
    if (n.flags.noHandshakeConnection || _session != _Session.open) {
      unawaited(_drainThenHandshake(HandshakeTrigger.noticeNoSession));
    } else if (n.flags.noHandshakeBoot) {
      unawaited(_ensureHandshake(HandshakeTrigger.noticeNoBootAnchor));
    }
    if (_resyncing) {
      _requestResyncBatch();
    } else {
      _maybeRequestBatch();
    }
    _evaluateSend();
  }

  void _onDongleError(SyncErrorCode code) {
    _log('DONGLE_ERROR', 'code=${code.code}');
    switch (code) {
      case SyncErrorCode.invalidTag || SyncErrorCode.staleChallenge:
        if (_handshakeFuture == null && !_stopped) {
          unawaited(_ensureHandshake(HandshakeTrigger.dongleRejected));
        }
      case SyncErrorCode.invalidAckSeq:
        break; // não reescrever aquele ACK
      case SyncErrorCode.noSession:
        _onNoSession();
      case SyncErrorCode.incompleteMessage || SyncErrorCode.resendFromBoundary:
        break;
    }
  }

  void _onNoSession() {
    _session = _Session.none;
    _awaitingBatch = false;
    final opening = _openingAck;
    if (opening != null && !_rewroteOpeningAck) {
      _rewroteOpeningAck = true;
      unawaited(
        _safe(() async {
          await dongle.sendAck(opening);
          _session = _Session.open;
          _maybeRequestBatch();
          _emit();
        }),
      );
    } else {
      unawaited(_drainThenHandshake(HandshakeTrigger.dongleNoSession));
    }
  }

  void _onBlock(SyncBlock b) {
    if (_session != _Session.open) return;
    final expected = (_lastRead ?? 0) + 1;
    final acceptable =
        b.seq == expected ||
        (b.marker == MarkerKind.boot &&
            b.fromSeq == expected &&
            b.seq > expected);
    if (_resyncing) {
      if (!acceptable) return;
      _resyncing = false;
    } else if (!acceptable) {
      if (b.seq <= (_lastRead ?? 0)) return; // duplicata
      _resync(SyncErrorCode.incompleteMessage);
      return;
    }
    if (!_buffer.add(BufferedBlock(block: b, readAt: _clock.now()))) {
      _log('BUFFER_FULL', 'seq=${b.seq}');
      _resync(SyncErrorCode.incompleteMessage);
      return;
    }
    _lastRead = b.lastCoveredSeq;
    _lastBlockAt = _clock.now();
    _log('BLOCK', 'seq=${b.seq} kind=${b.marker.name} buf=${_buffer.length}');
    if (_lastRead! >= _batchTarget) _awaitingBatch = false;
    _evaluateSend();
    _maybeRequestBatch();
  }

  // lotes

  void _maybeRequestBatch() {
    if (!_linkUp ||
        _session != _Session.open ||
        _resyncing ||
        _headSeq == null ||
        _lastRead == null ||
        _awaitingBatch ||
        _stopped) {
      return;
    }
    final k = min(min(_headSeq! - _lastRead!, _buffer.freeSlots), 0xFFFF);
    if (k <= 0) return;
    _awaitingBatch = true;
    _batchTarget = _lastRead! + k;
    _lastBlockAt = _clock.now();
    _log('BATCH_REQ', 'k=$k last=$_lastRead head=$_headSeq');
    unawaited(_safe(() => dongle.requestBatch(k)));
  }

  void _requestResyncBatch() {
    final head = _headSeq;
    final last = _lastRead;
    if (head == null || last == null) return;
    final k = min(min(head - last, _buffer.freeSlots), 0xFFFF);
    if (k <= 0) return;
    _awaitingBatch = true;
    _batchTarget = last + k;
    _lastBlockAt = _clock.now();
    _log('BATCH_REQ', 'k=$k last=$last head=$head resync=1');
    unawaited(_safe(() => dongle.requestBatch(k)));
  }

  void _resync(SyncErrorCode code) {
    if (_resyncing) return; // no máximo um erro por ressincronização
    _log('RESYNC', 'code=${code.code}');
    _resyncing = true;
    _awaitingBatch = false;
    _buffer.dropPending();
    _lastRead = _appAck ?? _lastRead;
    unawaited(
      _safe(() async {
        await dongle.sendError(code);
        _requestResyncBatch();
      }),
    );
  }

  Future<void> _safe(Future<void> Function() action) async {
    try {
      await action();
    } on Object catch (e) {
      _log('ACTION_FAILED', 'err=$e');
    }
  }

  // tick

  void _tick() {
    if (_disposed) return;
    final now = _clock.now();
    if (_awaitingBatch && now - _lastBlockAt >= kBatchStallTimeout) {
      _awaitingBatch = false;
      _maybeRequestBatch();
    }
    if (_resyncing && now - _lastBlockAt >= kBatchStallTimeout) {
      _requestResyncBatch();
    }
    if (_linkUp &&
        !_stopped &&
        _session == _Session.none &&
        _notBefore != null &&
        now >= _notBefore! &&
        _handshakeFuture == null) {
      unawaited(_ensureHandshake(_lastTrigger));
    }
    _evaluateSend();
    _emit();
  }

  // handshake

  Future<bool> _ensureHandshake(HandshakeTrigger trigger) {
    final running = _handshakeFuture;
    if (running != null) return running;
    return _handshakeFuture = _handshakeOnce(trigger).whenComplete(() {
      _handshakeFuture = null;
    });
  }

  Future<bool> _handshakeOnce(HandshakeTrigger trigger) async {
    final devId = _devId;
    if (!_linkUp || devId == null || _stopped) return false;
    final now = _clock.now();
    if (_notBefore != null && now < _notBefore!) return false;
    _lastTrigger = trigger;
    final g = _generation;
    _session = _Session.handshaking;
    _emit();
    try {
      final outcome = await handshake.run(
        devId,
        trigger: trigger,
        generation: g,
      );
      if (g != _generation || !_linkUp) {
        _log('HS_DISCARDED', 'gen=$g now=$_generation');
        return false;
      }
      _suggested = outcome.suggestedLatency;
      _openingAck = outcome.ack;
      _rewroteOpeningAck = false;
      _onAck(outcome.ack, fromRunner: true);
      _session = _Session.open;
      _backoff.reset();
      _notBefore = null;
      _nextAttemptAt = null;
      _lastError = null;
      _log('HS_OPEN', 'ack=$_appAck');
      final held = _heldAck;
      if (held != null) {
        _heldAck = null;
        _onAck(held);
      }
      _maybeRequestBatch();
      _evaluateSend();
      _emit();
      return true;
    } on HandshakeLinkLost {
      _log('HS_FAILED', 'reason=link_lost');
      if (g == _generation) _session = _Session.none;
      _emit();
      return false;
    } on SyncFailure catch (e) {
      if (g != _generation || !_linkUp) return false;
      _session = _Session.none;
      _onHandshakeFailure(e);
      return false;
    } on Object catch (e) {
      _session = _Session.none;
      _log('HS_FAILED', 'err=$e');
      _emit();
      return false;
    }
  }

  void _onHandshakeFailure(SyncFailure e) {
    final now = _clock.now();
    Duration delay;
    switch (e) {
      case DongleBlockedFailure():
        _stopped = true;
        delay = Duration.zero;
      case RateLimitedFailure(:final retryAfter):
        delay = retryAfter;
      case ChallengeUnavailable(:final retryAfter):
        delay = _maxDuration(_backoff.next(), retryAfter);
      case ServerUnavailableFailure(:final retryAfter):
        delay = _maxDuration(_backoff.next(), retryAfter);
      default:
        delay = _backoff.next();
    }
    _lastError = e.message;
    if (!_stopped) {
      _notBefore = now + delay;
      _nextAttemptAt = _wall().add(delay);
    }
    _log('HS_FAILED', 'err=${e.runtimeType} wait=${delay.inMilliseconds}');
    _emit();
  }

  Duration _maxDuration(Duration a, Duration? b) => b != null && b > a ? b : a;

  /// Posta os blocos pendentes antes de refazer o handshake, quando o token
  /// ainda vale: o handshake novo traria um `TAIL` maior e o servidor daria por
  /// perdido o que o app já leu.
  Future<void> _drainThenHandshake(HandshakeTrigger trigger) async {
    if (!_draining) {
      _draining = true;
      try {
        while (_buffer.pendingCount > 0 &&
            !_postInFlight &&
            !server.needsHandshake(_wall(), Duration.zero)) {
          final before = _buffer.length;
          await _post(min(_buffer.pendingCount, _lotLimit));
          if (_buffer.length >= before) break;
        }
      } finally {
        _draining = false;
      }
    }
    await _ensureHandshake(trigger);
  }

  // ACK e envio

  void _onAck(SyncFrame frame, {bool fromRunner = false}) {
    final a = AckInfo.fromValue(frame.value).ackSeq;
    _appAck = max(_appAck ?? a, a);
    _buffer.removeUpTo(_appAck!);
    _lastRead = max(_lastRead ?? _appAck!, _appAck!);
    if (fromRunner) return;
    if (_linkUp && _session == _Session.open) {
      unawaited(_safe(() => dongle.sendAck(frame)));
    } else {
      _heldAck = frame;
    }
  }

  void _evaluateSend() {
    if (_disposed || _stopped || _postInFlight) return;
    final n = SendPolicy(tLat: _effectiveLatency).lotSize(
      now: _clock.now(),
      buffer: _buffer,
      postInFlight: _postInFlight,
      flashHigh: _flags?.flashHigh ?? false,
      notBefore: _notBefore,
      currentLotLimit: _lotLimit,
    );
    if (n > 0) unawaited(_post(n));
  }

  Future<void> _post(int n) async {
    if (_postInFlight) return;
    _postInFlight = true;
    try {
      if (server.needsHandshake(_wall(), kTokenRenewMargin)) {
        final ok = await _ensureHandshake(HandshakeTrigger.tokenExpiring);
        if (!ok) return;
      }
      final lot = _buffer.takeForPost(min(n, _buffer.pendingCount));
      if (lot.isEmpty) return;
      _log('POST', 'n=${lot.length} first=${lot.first.block.seq}');
      final result = await server.postBlocks([
        for (final b in lot) b.block.frame,
      ]);
      _onPostResult(result, lot);
    } finally {
      _postInFlight = false;
      _emit();
      _evaluateSend();
      _maybeRequestBatch();
    }
  }

  void _onPostResult(PostResult result, List<BufferedBlock> lot) {
    final now = _clock.now();
    switch (result) {
      case BlocksAccepted(:final ack):
        _postsOk++;
        final acked = AckInfo.fromValue(ack.value).ackSeq;
        _onAck(ack);
        _buffer.releaseInFlight();
        var stalled = false;
        if (acked < lot.last.block.lastCoveredSeq) {
          _interruptedCount = _interruptedAck == acked
              ? _interruptedCount + 1
              : 1;
          _interruptedAck = acked;
          _log('POST_INTERRUPTED', 'ack=$acked count=$_interruptedCount');
          if (_interruptedCount >= 2) {
            _interruptedCount = 0;
            _interruptedAck = null;
            _resync(SyncErrorCode.resendFromBoundary);
            // Espera crescente: sem ela, um servidor que nunca avança em
            // repetiria o ciclo sem pausa.
            _notBefore = now + _backoff.next();
            stalled = true;
          }
        } else {
          _interruptedCount = 0;
          _interruptedAck = null;
        }
        _lotLimit = kMaxBlocksPerPost;
        if (!stalled) {
          _notBefore = null;
          _nextAttemptAt = null;
          _backoff.reset();
        }
      case Unauthorized() || Forbidden():
        _postsFailed++;
        _buffer.releaseInFlight();
        server.clearToken();
        _lastTrigger = HandshakeTrigger.unauthorized;
        unawaited(_ensureHandshake(HandshakeTrigger.unauthorized));
      case DongleBlocked(:final reason):
        _postsFailed++;
        _buffer.releaseInFlight();
        _stopped = true;
        _lastError = 'Dongle bloqueado no servidor ($reason)';
      case RateLimited(:final retryAfter):
        _postsFailed++;
        _buffer.releaseInFlight();
        _notBefore = now + retryAfter;
        _lotLimit = max(1, _lotLimit ~/ 2);
      case PostUnavailable(:final retryAfter):
        _postsFailed++;
        _buffer.releaseInFlight();
        final wait = _maxDuration(_backoff.next(), retryAfter);
        _notBefore = now + wait;
        _nextAttemptAt = _wall().add(wait);
        _lotLimit = max(1, _lotLimit ~/ 2);
      case MalformedRequest(:final code):
        _postsFailed++;
        _buffer.releaseInFlight();
        _lastError = 'Servidor recusou o lote ($code)';
        _resync(SyncErrorCode.resendFromBoundary);
    }
  }
}
