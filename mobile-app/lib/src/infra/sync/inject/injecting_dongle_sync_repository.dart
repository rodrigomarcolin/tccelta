import 'dart:async';

import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';
import 'package:tccelta_mobile/src/domain/repositories/dongle_sync_repository.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_block.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_frame.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_protocol.dart';
import 'package:tccelta_mobile/src/infra/sync/inject/block_forger.dart';

/// Decorator do canal SYNC que reescreve só os blocos de dados (marcador
/// `none`) com telemetria forjada, deixando o GPS intacto. Handshake,
/// marcadores (boot/loss), avisos, ACKs e erros passam sem alteração — o que
/// preserva a continuidade de `seq` e o handshake real do dongle.
///
/// O callback `nextSample` devolve a próxima amostra PID→valor, ou `null`
/// quando a injeção está desligada (então o bloco real segue intacto).
class InjectingDongleSyncRepository implements DongleSyncRepository {
  /// Envolve o repo [_inner] e consulta [_nextSample] a cada bloco de dados.
  InjectingDongleSyncRepository(this._inner, this._nextSample) {
    _sub = _inner.events.listen(
      _onEvent,
      onError: _events.addError,
      onDone: _events.close,
    );
  }

  final DongleSyncRepository _inner;
  final Map<Obd2Pid, double>? Function() _nextSample;
  final StreamController<DongleSyncEvent> _events =
      StreamController<DongleSyncEvent>.broadcast();
  StreamSubscription<DongleSyncEvent>? _sub;

  void _onEvent(DongleSyncEvent e) {
    if (_events.isClosed) return;
    _events.add(_maybeForge(e));
  }

  DongleSyncEvent _maybeForge(DongleSyncEvent e) {
    if (e is! BlockReceived || e.block.marker != MarkerKind.none) return e;
    final sample = _nextSample();
    if (sample == null) return e;
    try {
      return BlockReceived(BlockForger.forge(e.block, sample));
    } on Object {
      return e; // bloco real inalterado se a forja falhar
    }
  }

  @override
  Stream<DongleSyncEvent> get events => _events.stream;

  @override
  bool get isLinkUp => _inner.isLinkUp;

  @override
  int get linkGeneration => _inner.linkGeneration;

  @override
  int get mtu => _inner.mtu;

  @override
  Future<void> sendChallenge(SyncFrame challenge) =>
      _inner.sendChallenge(challenge);

  @override
  Future<void> sendAck(SyncFrame ack) => _inner.sendAck(ack);

  @override
  Future<void> requestBatch(int k) => _inner.requestBatch(k);

  @override
  Future<void> sendError(SyncErrorCode code) => _inner.sendError(code);

  /// Cancela a escuta do repo interno e fecha o stream mapeado.
  Future<void> dispose() async {
    await _sub?.cancel();
    _sub = null;
    await _events.close();
  }
}
