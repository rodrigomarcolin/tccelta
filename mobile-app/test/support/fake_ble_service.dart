import 'dart:async';

import 'package:tccelta_mobile/src/domain/ble/ble_adapter_state.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_device.dart';
import 'package:tccelta_mobile/src/services/ble/ble_service.dart';

/// [BleService] falso para exercitar as camadas sem hardware BLE.
///
/// Como só o adapter conhece a lib real, trocar apenas `bleServiceProvider` por
/// este fake permite rodar telas e view models em testes/dev.
class FakeBleService implements BleService {
  /// Cria o fake com os [devices], estados de adaptador e sequência de fases.
  FakeBleService({
    this.devices = const [],
    this.adapterStates = const [BleAdapterState.on],
    this.scanError,
    this.holdScanOpen = false,
    this.phases = const [
      BleConnectionPhase.connecting,
      BleConnectionPhase.optimizingLink,
      BleConnectionPhase.discovering,
      BleConnectionPhase.enablingNotify,
      BleConnectionPhase.ready,
    ],
  });

  /// Dongles emitidos pelo scan.
  final List<BleDevice> devices;

  /// Estados de adaptador emitidos.
  final List<BleAdapterState> adapterStates;

  /// Se não nulo, o scan emite este erro em vez dos [devices].
  final Object? scanError;

  /// Quando `true`, o scan emite os [devices] mas NÃO fecha o stream — imita um
  /// scan real que segue ativo até o timeout/stop (o default fecha na hora,
  /// para manter os testes de scan único simples).
  final bool holdScanOpen;

  /// Sequência de fases que a conexão falsa percorre.
  final List<BleConnectionPhase> phases;

  /// Se há um scan ativo (não parado). Modela o contrato do FBP: um scan por
  /// vez — o anterior precisa ser parado antes de o novo render resultados.
  bool _scanActive = false;

  /// Quantas vezes [scan] foi chamado (para asserir coalescência de rescans).
  int scanCount = 0;

  @override
  Stream<BleAdapterState> get adapterState =>
      Stream<BleAdapterState>.fromIterable(adapterStates);

  @override
  Stream<List<BleDevice>> scan({
    List<String> withServiceUuids = const [],
    List<String> withNames = const [],
    Duration timeout = const Duration(seconds: 15),
  }) {
    scanCount++;
    if (scanError != null) {
      return Stream<List<BleDevice>>.error(scanError!);
    }
    // Sempre via controller (não `Stream.value`): assim o cancel após concluído
    // se comporta bem sob o FakeAsync dos widget tests, e o adapter real também
    // fecha a stream ao fim do scan (ver ble_plus). Por padrão é disparo único
    // (fecha após emitir); com [holdScanOpen] segue aberto imitando um scan em
    // andamento até o timeout/stop.
    final controller = StreamController<List<BleDevice>>();
    controller
      ..onListen = () {
        // Scan anterior ainda ativo (teardown não drenado) -> nada é achado.
        controller.add(_scanActive ? const [] : devices);
        if (holdScanOpen) {
          _scanActive = true;
        } else {
          unawaited(controller.close());
        }
      }
      ..onCancel = () async {
        // Reset deferido (microtask, sem Timer) espelhando o `stopScan()`
        // assíncrono real: só drena quando o cancel é aguardado.
        await Future<void>.microtask(() {});
        _scanActive = false;
      };
    return controller.stream;
  }

  @override
  Future<void> stopScan() async {
    _scanActive = false;
  }

  @override
  Future<BleConnection> connect({
    required String deviceId,
    required String serviceUuid,
    required String rxUuid,
    required String txUuid,
    int mtu = 512,
  }) async =>
      FakeBleConnection(phases);
}

/// Conexão falsa que percorre uma sequência de fases, uma por turno do loop de
/// eventos (para que os assinantes tenham tempo de se registrar).
class FakeBleConnection implements BleConnection {
  /// Cria a conexão e começa a emitir a sequência de fases recebida.
  FakeBleConnection(this._phases) {
    unawaited(_run());
  }

  final List<BleConnectionPhase> _phases;
  final StreamController<BleConnectionPhase> _phaseCtrl =
      StreamController<BleConnectionPhase>.broadcast();
  final StreamController<List<int>> _incomingCtrl =
      StreamController<List<int>>.broadcast();
  BleConnectionPhase _current = BleConnectionPhase.idle;

  Future<void> _run() async {
    for (final p in _phases) {
      await Future<void>.delayed(Duration.zero);
      _current = p;
      if (!_phaseCtrl.isClosed) _phaseCtrl.add(p);
    }
  }

  @override
  Stream<BleConnectionPhase> get phase => _phaseCtrl.stream;

  @override
  BleConnectionPhase get currentPhase => _current;

  @override
  Stream<List<int>> get incoming => _incomingCtrl.stream;

  @override
  bool get isReady => _current == BleConnectionPhase.ready;

  @override
  Future<void> write(List<int> bytes) async {}

  @override
  Future<void> disconnect() async {
    _current = BleConnectionPhase.disconnected;
    await _phaseCtrl.close();
    await _incomingCtrl.close();
  }
}
