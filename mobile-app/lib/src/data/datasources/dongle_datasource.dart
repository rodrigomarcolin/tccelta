import 'package:tccelta_mobile/src/domain/ble/ble_adapter_state.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_device.dart';
import 'package:tccelta_mobile/src/services/ble/ble_service.dart';

/// Config específica do dongle (contrato do firmware — ver `CLAUDE.md`).
///
/// BLE via Nordic UART Service: o app escreve comandos ELM327 no RX e recebe
/// respostas por notify no TX.
abstract final class DongleConfig {
  /// Nome anunciado pelo dongle.
  static const String advertisedName = 'OBD2Dongle';

  /// Service UUID do NUS.
  static const String service = '6E400001-B5A3-F393-E0A9-E50E24DCCA9E';

  /// RX (WRITE) — app -> dongle (comandos ELM327).
  static const String rx = '6E400002-B5A3-F393-E0A9-E50E24DCCA9E';

  /// TX (NOTIFY) — dongle -> app (respostas ELM327).
  static const String tx = '6E400003-B5A3-F393-E0A9-E50E24DCCA9E';
}

/// Envelopa o [BleService] genérico com o que é específico do dongle (nome +
/// UUIDs da NUS). Não guarda estado — o repository é quem faz isso.
class DongleDatasource {
  /// Cria o datasource sobre um [BleService].
  DongleDatasource(this._ble);

  final BleService _ble;

  /// Estado do adaptador Bluetooth do telefone.
  Stream<BleAdapterState> get adapterState => _ble.adapterState;

  /// Escaneia apenas por dongles (filtra por serviço NUS + nome anunciado).
  Stream<List<BleDevice>> scanForDongles({
    Duration timeout = const Duration(seconds: 15),
  }) =>
      _ble.scan(
        withServiceUuids: const [DongleConfig.service],
        withNames: const [DongleConfig.advertisedName],
        timeout: timeout,
      );

  /// Para o scan em andamento.
  Future<void> stopScan() => _ble.stopScan();

  /// Conecta ao dongle [deviceId] com os UUIDs da NUS.
  Future<BleConnection> connect(String deviceId) => _ble.connect(
        deviceId: deviceId,
        serviceUuid: DongleConfig.service,
        rxUuid: DongleConfig.rx,
        txUuid: DongleConfig.tx,
      );
}
