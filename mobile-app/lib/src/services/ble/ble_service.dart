import 'package:tccelta_mobile/src/domain/ble/ble_adapter_state.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_device.dart';

/// Port BLE genérico (categoria `services` do bootstrap): fala BLE puro, sem
/// saber nada de "dongle OBD2". Só o adapter (`infra`) conhece a lib concreta.
///
/// Trocar de biblioteca BLE = escrever outro adapter e trocar um único
/// provider; as camadas acima (data/ui) não mudam.
abstract interface class BleService {
  /// Estado do adaptador Bluetooth do telefone.
  Stream<BleAdapterState> get adapterState;

  /// Escaneia e emite a lista corrente de dispositivos que casam com os
  /// filtros. Emite uma nova lista a cada atualização de resultados.
  Stream<List<BleDevice>> scan({
    List<String> withServiceUuids,
    List<String> withNames,
    Duration timeout,
  });

  /// Para o scan em andamento.
  Future<void> stopScan();

  /// Conecta e prepara a conexão (MTU + discover + notify) de forma resiliente
  /// a reconexão. Retorna o handle IMEDIATAMENTE; o progresso vem por
  /// [BleConnection.phase].
  Future<BleConnection> connect({
    required String deviceId,
    required String serviceUuid,
    required String rxUuid,
    required String txUuid,
    int mtu,
  });
}
