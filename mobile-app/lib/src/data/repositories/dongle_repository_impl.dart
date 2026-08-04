import 'dart:async';

import 'package:tccelta_mobile/src/core/errors/ble_failure.dart';
import 'package:tccelta_mobile/src/data/datasources/dongle_datasource.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_adapter_state.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_device.dart';
import 'package:tccelta_mobile/src/domain/repositories/dongle_repository.dart';

/// Impl do [DongleRepository] — *source of truth* da conexão.
///
/// Guarda a conexão ativa, re-publica a stream de fases (para que os view
/// models não dependam do handle direto) e converte erros crus em `Failure`.
class DongleRepositoryImpl implements DongleRepository {
  /// Cria o repository sobre um [DongleDatasource].
  DongleRepositoryImpl(this._ds);

  final DongleDatasource _ds;

  BleConnection? _conn;
  StreamSubscription<BleConnectionPhase>? _phaseSub;
  final StreamController<BleConnectionPhase> _phaseCtrl =
      StreamController<BleConnectionPhase>.broadcast();

  @override
  Stream<BleAdapterState> get adapterState => _ds.adapterState;

  @override
  Stream<List<BleDevice>> scan({
    Duration timeout = const Duration(seconds: 15),
  }) =>
      _ds.scanForDongles(timeout: timeout).handleError(
            (Object e) => throw BleScanFailure('Falha no scan BLE', cause: e),
          );

  @override
  Future<void> stopScan() => _ds.stopScan();

  @override
  Future<void> connect(String deviceId) async {
    try {
      final conn = await _ds.connect(deviceId);
      _conn = conn;
      await _phaseSub?.cancel();
      // Assina ANTES de retornar: as fases (inclusive ready/failed) são
      // re-emitidas no _phaseCtrl para os view models.
      _phaseSub = conn.phase.listen(
        _phaseCtrl.add,
        onError: (Object e) => _phaseCtrl.addError(
          BleConnectionFailure('Erro de conexão', cause: e),
        ),
      );
    } on Object catch (e) {
      throw BleConnectionFailure('Não foi possível conectar', cause: e);
    }
  }

  @override
  Stream<BleConnectionPhase> get connectionPhase => _phaseCtrl.stream;

  @override
  BleConnection? get connection => _conn;

  @override
  bool get isReady => _conn?.isReady ?? false;

  @override
  Future<void> disconnect() async {
    await _phaseSub?.cancel();
    _phaseSub = null;
    await _conn?.disconnect();
    _conn = null;
  }
}
