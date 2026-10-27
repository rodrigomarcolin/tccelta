import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:tccelta_mobile/src/core/errors/sync_failure.dart';
import 'package:tccelta_mobile/src/core/time/monotonic_clock.dart';
import 'package:tccelta_mobile/src/data/datasources/dongle_datasource.dart';
import 'package:tccelta_mobile/src/data/datasources/sync_ble_datasource.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';
import 'package:tccelta_mobile/src/domain/repositories/dongle_repository.dart';
import 'package:tccelta_mobile/src/domain/repositories/dongle_sync_repository.dart';
import 'package:tccelta_mobile/src/domain/sync/handshake_messages.dart';
import 'package:tccelta_mobile/src/domain/sync/new_block_notice.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_block.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_frame.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_protocol.dart';

/// *Source of truth* do canal SYNC com o dongle.
///
/// Obtém a [BleConnection] viva do [DongleRepository] e, a cada fase, confere
/// a identidade da conexão: conexão nova leva a um novo [SyncBleDatasource]
/// sobre o canal SYNC dela. Mensagens que chegam antes da fase `ready` (o
/// aviso é enviado dentro do próprio `onSubscribe`) ficam retidas e saem logo
/// depois do `SyncLinkChanged(up: true)`.
class DongleSyncRepositoryImpl implements DongleSyncRepository {
  /// Cria o repository sobre o [_dongle].
  DongleSyncRepositoryImpl(this._dongle, {MonotonicClock? clock})
    : _clock = clock ?? SystemMonotonicClock() {
    _phaseSub = _dongle.connectionPhase.listen(
      _onPhase,
      onError: (_) => _onPhase(BleConnectionPhase.failed),
    );
    _bind();
  }

  static const int _maxHeld = 32;

  final DongleRepository _dongle;
  final MonotonicClock _clock;
  final StreamController<DongleSyncEvent> _events =
      StreamController<DongleSyncEvent>.broadcast();
  final List<DongleSyncEvent> _held = [];

  StreamSubscription<BleConnectionPhase>? _phaseSub;
  StreamSubscription<SyncInbound>? _inboundSub;
  SyncBleDatasource? _datasource;
  BleConnection? _bound;
  bool _linkUp = false;
  int _generation = 0;

  @override
  Stream<DongleSyncEvent> get events => _events.stream;

  @override
  bool get isLinkUp => _linkUp;

  @override
  int get linkGeneration => _generation;

  @override
  int get mtu => _dongle.connection?.mtu ?? 23;

  void _emit(DongleSyncEvent e) {
    if (!_events.isClosed) _events.add(e);
  }

  /// Vincula o datasource à conexão ATUAL, recriando-o se a identidade mudou.
  void _bind() {
    final conn = _dongle.connection;
    if (identical(conn, _bound)) return;
    _unbind();
    _bound = conn;
    final channel = conn?.channel(SyncConfig.service);
    if (channel == null) return;
    final ds = SyncBleDatasource(channel, clock: _clock);
    _datasource = ds;
    _inboundSub = ds.inbound.listen(_onInbound);
  }

  void _unbind() {
    unawaited(_inboundSub?.cancel());
    _inboundSub = null;
    _datasource?.dispose();
    _datasource = null;
    _bound = null;
    _held.clear();
    _setDown();
  }

  void _setDown() {
    if (!_linkUp) return;
    _linkUp = false;
    _emit(SyncLinkChanged(up: false, generation: _generation));
  }

  void _onPhase(BleConnectionPhase phase) {
    _bind();
    switch (phase) {
      case BleConnectionPhase.ready:
        final channel = _bound?.channel(SyncConfig.service);
        if (channel != null && channel.isReady && !_linkUp) {
          _linkUp = true;
          _generation++;
          _emit(SyncLinkChanged(up: true, generation: _generation));
          _held
            ..forEach(_emit)
            ..clear();
        }
      case BleConnectionPhase.reconnecting ||
          BleConnectionPhase.disconnected ||
          BleConnectionPhase.failed:
        _held.clear();
        _setDown();
      case _:
        break;
    }
  }

  void _onInbound(SyncInbound inbound) {
    final event = _toEvent(inbound);
    if (event == null) return;
    if (_linkUp) {
      _emit(event);
    } else if (_held.length < _maxHeld) {
      _held.add(event);
    }
  }

  DongleSyncEvent? _toEvent(SyncInbound inbound) {
    switch (inbound) {
      case InboundIncomplete(:final kind):
        return TransportIncomplete(kind);
      case InboundFrame(:final frame):
        try {
          return switch (frame.type) {
            SyncMessageType.notice => NoticeReceived(
              NewBlockNotice.fromValue(frame.value),
            ),
            SyncMessageType.block => BlockReceived(SyncBlock.fromFrame(frame)),
            SyncMessageType.response => ResponseReceived(
              frame,
              ResponseInfo.fromValue(frame.value),
            ),
            SyncMessageType.error => _errorEvent(frame),
            _ => null,
          };
        } on FormatException catch (e) {
          if (kDebugMode) debugPrint('SYNC descartou mensagem: ${e.message}');
          return null;
        }
    }
  }

  DongleSyncEvent? _errorEvent(SyncFrame frame) {
    if (frame.value.length != 1) return null;
    final code = SyncErrorCode.fromCode(frame.value[0]);
    return code == null ? null : DongleErrorReceived(code);
  }

  Future<void> _send(SyncFrame frame) async {
    final ds = _datasource;
    if (ds == null || !_linkUp) {
      throw const SyncLinkFailure('Canal SYNC indisponível');
    }
    try {
      await ds.send(frame);
    } on Object catch (e) {
      throw SyncLinkFailure('Falha ao escrever no canal SYNC', cause: e);
    }
  }

  @override
  Future<void> sendChallenge(SyncFrame challenge) {
    final n = challenge.value.length;
    if (challenge.type != SyncMessageType.challenge ||
        (n != kChallengeLength && n != kChallengeDiagLength)) {
      throw ArgumentError.value(challenge, 'challenge', 'não é um desafio');
    }
    return _send(challenge);
  }

  @override
  Future<void> sendAck(SyncFrame ack) {
    if (ack.type != SyncMessageType.ack || ack.value.length != kAckLength) {
      throw ArgumentError.value(ack, 'ack', 'não é um ACK');
    }
    return _send(ack);
  }

  @override
  Future<void> requestBatch(int k) => _send(SyncFrame.batchRequest(k));

  @override
  Future<void> sendError(SyncErrorCode code) {
    if (code != SyncErrorCode.incompleteMessage &&
        code != SyncErrorCode.resendFromBoundary) {
      throw ArgumentError.value(code, 'code', 'só 0x04 e 0x07 são enviados');
    }
    return _send(SyncFrame.error(code));
  }

  /// Cancela as escutas; chamado quando o provider é descartado.
  void dispose() {
    unawaited(_phaseSub?.cancel());
    _phaseSub = null;
    _unbind();
    unawaited(_events.close());
  }
}
