import 'package:tccelta_mobile/src/domain/ble/ble_adapter_state.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_bond_state.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_device.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_signal_level.dart';
import 'package:tccelta_mobile/src/services/ble/ble_service.dart';

/// Config específica do dongle (contrato do firmware — ver `CLAUDE.md`).
///
/// BLE via Nordic UART Service: o app escreve comandos ELM327 no RX e recebe
/// respostas por notify no TX.
abstract final class DongleConfig {
  /// Service UUID do NUS.
  static const String service = '6E400001-B5A3-F393-E0A9-E50E24DCCA9E';

  /// RX (WRITE) — app -> dongle (comandos ELM327).
  static const String rx = '6E400002-B5A3-F393-E0A9-E50E24DCCA9E';

  /// TX (NOTIFY) — dongle -> app (respostas ELM327).
  static const String tx = '6E400003-B5A3-F393-E0A9-E50E24DCCA9E';
}

/// Config do serviço SYNC do dongle (TLV de telemetria autenticada).
abstract final class SyncConfig {
  /// Service UUID do SYNC.
  static const String service = '7A3C0001-9B1E-4F2A-8C5D-2E6F1A0B3C4D';

  /// RX (WRITE) — app -> dongle.
  static const String rx = '7A3C0002-9B1E-4F2A-8C5D-2E6F1A0B3C4D';

  /// TX (NOTIFY) — dongle -> app.
  static const String tx = '7A3C0003-9B1E-4F2A-8C5D-2E6F1A0B3C4D';
}

/// Envelopa o [BleService] genérico com o que é específico do dongle (nome +
/// UUIDs da NUS). Não guarda estado — o repository é quem faz isso.
class DongleDatasource {
  /// Cria o datasource sobre um [BleService].
  DongleDatasource(this._ble);

  final BleService _ble;

  /// Estado do adaptador Bluetooth do telefone.
  Stream<BleAdapterState> get adapterState => _ble.adapterState;

  /// Escaneia apenas por dongles, filtrando **somente pelo Service UUID da
  /// NUS** — o nome anunciado não é usado como filtro (ele pode mudar no
  /// firmware e o SO tende a cachear o nome antigo por MAC).
  ///
  /// Usa `continuousUpdates` para o RSSI atualizar em tempo real durante o scan
  /// e classifica cada dispositivo com [BleSignalLevel] (regra do domain) —
  /// aqui é o único ponto que aplica a classificação; a UI só exibe.
  Stream<List<BleDevice>> scanForDongles({
    Duration timeout = const Duration(seconds: 15),
  }) => _ble
      .scan(
        withServiceUuids: const [DongleConfig.service],
        timeout: timeout,
        continuousUpdates: true,
      )
      .map(
        (devices) => devices
            .map(
              (d) => d.copyWith(signal: BleSignalLevel.fromRssi(d.rssi)),
            )
            .toList(growable: false),
      );

  /// Para o scan em andamento.
  Future<void> stopScan() => _ble.stopScan();

  /// Se o SO deixa o app consultar, criar e remover bonds (Android).
  bool get canManageBonds => _ble.canManageBonds;

  /// Estado do bond do SO com [deviceId].
  Future<BleBondState> bondState(String deviceId) => _ble.bondState(deviceId);

  /// Remove o bond do SO com [deviceId] (Android).
  Future<void> removeBond(String deviceId) => _ble.removeBond(deviceId);

  /// Acompanha os anúncios de UM dongle (filtro por id), para saber quando a
  /// janela de pareamento abre. Emite `null` enquanto o dongle não foi visto.
  Stream<BleDevice?> watchDongle(
    String deviceId, {
    Duration timeout = const Duration(seconds: 120),
  }) => _ble
      .scan(
        withRemoteIds: [deviceId],
        timeout: timeout,
        continuousUpdates: true,
      )
      .map((devices) {
        for (final d in devices) {
          if (d.id == deviceId) return d;
        }
        return null;
      });

  /// Prazo do pareamento/cifra: menor que o `PAIR_AUTH_TIMEOUT_MS` do firmware
  /// (35 s), para o app desistir antes de o dongle derrubar a conexão.
  static const Duration securingTimeout = Duration(seconds: 30);

  /// Conecta ao dongle [deviceId] com os UUIDs da NUS e abre também o canal
  /// do serviço SYNC (ausente em firmware antigo sem derrubar a conexão).
  Future<BleConnection> connect(String deviceId) => _ble.connect(
    deviceId: deviceId,
    serviceUuid: DongleConfig.service,
    rxUuid: DongleConfig.rx,
    txUuid: DongleConfig.tx,
    extraChannels: const [
      BleChannelSpec(
        serviceUuid: SyncConfig.service,
        rxUuid: SyncConfig.rx,
        txUuid: SyncConfig.tx,
      ),
    ],
    securingTimeout: securingTimeout,
  );
}
