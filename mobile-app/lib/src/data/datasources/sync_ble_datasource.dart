import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:tccelta_mobile/src/core/time/monotonic_clock.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';
import 'package:tccelta_mobile/src/domain/sync/fragment_assembler.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_frame.dart';

/// Entrada do canal SYNC já remontada.
@immutable
sealed class SyncInbound {
  const SyncInbound();
}

/// Mensagem TLV completa.
final class InboundFrame extends SyncInbound {
  /// Cria a entrada.
  const InboundFrame(this.frame);

  /// Frame remontado.
  final SyncFrame frame;
}

/// Mensagem que não pôde ser remontada.
final class InboundIncomplete extends SyncInbound {
  /// Cria a entrada.
  const InboundIncomplete(this.kind);

  /// Motivo.
  final AssemblyErrorKind kind;
}

/// Envolve o canal SYNC: remonta fragmentos do TX e escreve TLVs no RX.
class SyncBleDatasource {
  /// Cria o datasource e passa a escutar o [_channel].
  SyncBleDatasource(this._channel, {MonotonicClock? clock})
    : _clock = clock ?? SystemMonotonicClock() {
    _sub = _channel.incoming.listen(_onBytes);
  }

  static const Duration _timeoutPoll = Duration(milliseconds: 500);

  final BleChannel _channel;
  final MonotonicClock _clock;
  final FragmentAssembler _assembler = FragmentAssembler();
  final StreamController<SyncInbound> _ctrl =
      StreamController<SyncInbound>.broadcast();
  late final StreamSubscription<List<int>> _sub;
  Timer? _timer;

  /// Mensagens remontadas e falhas de montagem (broadcast).
  Stream<SyncInbound> get inbound => _ctrl.stream;

  /// Escreve [frame] (`T | L | V`) no RX, com resposta.
  Future<void> send(SyncFrame frame) => _channel.write(frame.toTlv());

  void _onBytes(List<int> bytes) {
    _assembler.add(bytes, _clock.now()).forEach(_emit);
    _syncTimer();
  }

  void _emit(AssemblyOutcome outcome) {
    switch (outcome) {
      case AssemblyPending():
        break;
      case AssemblyComplete(:final tlv):
        try {
          _add(InboundFrame(SyncFrame.parseTlv(Uint8List.fromList(tlv))));
        } on FormatException catch (e) {
          // Enquadramento íntegro, mas tipo desconhecido: não é erro 0x04.
          if (kDebugMode) debugPrint('SYNC descartou mensagem: ${e.message}');
        }
      case AssemblyError(:final kind):
        _add(InboundIncomplete(kind));
    }
  }

  void _add(SyncInbound event) {
    if (!_ctrl.isClosed) _ctrl.add(event);
  }

  /// Liga o relógio de *timeout* só enquanto há montagem em curso.
  void _syncTimer() {
    if (_assembler.assembling) {
      _timer ??= Timer.periodic(_timeoutPoll, (_) {
        final o = _assembler.checkTimeout(_clock.now());
        if (o != null) _emit(o);
        _syncTimer();
      });
    } else {
      _timer?.cancel();
      _timer = null;
    }
  }

  /// Libera a escuta e o temporizador.
  void dispose() {
    _timer?.cancel();
    _timer = null;
    unawaited(_sub.cancel());
    unawaited(_ctrl.close());
  }
}
