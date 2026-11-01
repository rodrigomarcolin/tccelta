import 'dart:async';

import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';
import 'package:tccelta_mobile/src/domain/location/simulated_gps_payload.dart';
import 'package:tccelta_mobile/src/services/ble/ble_service.dart';

/// Envelopa o [BleService] para receber as notificações cruas do simulador de
/// GPS ([SimulatedGpsContract]). Não guarda estado entre assinaturas.
class SimulatedGpsDatasource {
  /// Cria o datasource sobre um [BleService].
  SimulatedGpsDatasource(
    this._ble, {
    this.scanTimeout = const Duration(seconds: 8),
    this.minBackoff = const Duration(seconds: 2),
    this.maxBackoff = const Duration(seconds: 10),
  });

  final BleService _ble;

  /// Duração máxima de cada busca pelo simulador.
  final Duration scanTimeout;

  /// Espera antes da primeira nova tentativa; dobra a cada falha seguida.
  final Duration minBackoff;

  /// Teto da espera entre tentativas.
  final Duration maxBackoff;

  /// Notificações do simulador, cruas. Ao assinar, busca o simulador, conecta
  /// sem bond e repassa cada notificação; se não o acha ou o link cai, emite
  /// um erro e tenta de novo após a espera ([minBackoff] → [maxBackoff],
  /// reiniciada depois de uma conexão pronta). Cancelar a assinatura para a
  /// busca e desconecta.
  Stream<List<int>> payloads() {
    late StreamController<List<int>> out;
    late _SimulatorLink link;
    out = StreamController<List<int>>(
      onListen: () {
        link = _SimulatorLink(this, out);
        unawaited(link.run());
      },
      onCancel: () => link.stop(),
    );
    return out.stream;
  }
}

/// Um ciclo de busca → conexão → repasse, repetido até [stop].
class _SimulatorLink {
  _SimulatorLink(this._ds, this._out);

  final SimulatedGpsDatasource _ds;
  final StreamController<List<int>> _out;
  final Completer<void> _stopped = Completer<void>();
  final Completer<void> _finished = Completer<void>();
  Timer? _sleep;

  bool get _isStopped => _stopped.isCompleted;

  Future<void> run() async {
    var delay = _ds.minBackoff;
    try {
      while (!_isStopped) {
        try {
          final id = await _find();
          if (_isStopped) break;
          if (id == null) {
            _out.addError(
              StateError('Simulador ${SimulatedGpsContract.name} não achado'),
            );
          } else {
            final end = await _session(id);
            if (_isStopped) break;
            if (end.reachedReady) delay = _ds.minBackoff;
            _out.addError(StateError(end.message));
          }
        } on Object catch (e, st) {
          if (_isStopped) break;
          _out.addError(e, st);
        }
        await _wait(delay);
        final next = delay * 2;
        delay = next > _ds.maxBackoff ? _ds.maxBackoff : next;
      }
    } finally {
      _finished.complete();
    }
  }

  Future<void> stop() async {
    if (!_isStopped) _stopped.complete();
    _sleep?.cancel();
    await _finished.future;
  }

  /// Id do primeiro dispositivo achado, ou `null` ao fim da busca sem nenhum.
  Future<String?> _find() async {
    final found = Completer<String?>();
    final sub = _ds._ble
        .scan(
          // Os filtros são "OU": o UUID do serviço identifica o simulador;
          // o nome é só alternativa, pois o anúncio pode trazer outro nome.
          withServiceUuids: const [SimulatedGpsContract.serviceUuid],
          withNames: const [SimulatedGpsContract.name],
          timeout: _ds.scanTimeout,
        )
        .listen(
          (devices) {
            if (devices.isNotEmpty && !found.isCompleted) {
              found.complete(devices.first.id);
            }
          },
          onError: (Object e, StackTrace st) {
            if (!found.isCompleted) found.completeError(e, st);
          },
          onDone: () {
            if (!found.isCompleted) found.complete(null);
          },
        );
    try {
      return await Future.any([
        found.future,
        _stopped.future.then((_) => null),
      ]);
    } finally {
      // Sem aguardar o cancelamento: a parada explícita encerra o scan.
      unawaited(sub.cancel());
      await _ds._ble.stopScan();
    }
  }

  /// Conecta a [deviceId] e repassa as notificações até o link falhar ou
  /// [stop]; sempre desconecta ao sair.
  Future<_SessionEnd> _session(String deviceId) async {
    final conn = await _ds._ble.connect(
      deviceId: deviceId,
      serviceUuid: SimulatedGpsContract.serviceUuid,
      rxUuid: SimulatedGpsContract.rxUuid,
      txUuid: SimulatedGpsContract.txUuid,
      requireBond: false,
    );
    var reachedReady = conn.currentPhase == BleConnectionPhase.ready;
    final ended = Completer<void>();
    void end() {
      if (!ended.isCompleted) ended.complete();
    }

    final data = conn.incoming.listen(_out.add, onError: _out.addError);
    final phases = conn.phase.listen((p) {
      if (p == BleConnectionPhase.ready) reachedReady = true;
      if (p == BleConnectionPhase.failed ||
          p == BleConnectionPhase.disconnected) {
        end();
      }
    }, onDone: end);
    if (conn.currentPhase == BleConnectionPhase.failed) end();
    try {
      await Future.any([ended.future, _stopped.future]);
    } finally {
      unawaited(data.cancel());
      unawaited(phases.cancel());
      try {
        await conn.disconnect();
      } on Object {
        // Melhor esforço: o link pode já ter caído.
      }
    }
    final cause = conn.lastError;
    final suffix = cause == null ? '' : ' (${cause.name})';
    return _SessionEnd(
      reachedReady: reachedReady,
      message: reachedReady
          ? 'Conexão com o simulador caiu$suffix'
          : 'Falha ao conectar ao simulador$suffix',
    );
  }

  /// Espera [d] ou até [stop], o que vier antes.
  Future<void> _wait(Duration d) {
    if (_isStopped) return Future<void>.value();
    final done = Completer<void>();
    _sleep = Timer(d, done.complete);
    return Future.any([done.future, _stopped.future]);
  }
}

/// Como terminou uma sessão conectada.
class _SessionEnd {
  const _SessionEnd({required this.reachedReady, required this.message});

  final bool reachedReady;
  final String message;
}
