import 'package:tccelta_mobile/src/domain/ble/ble_adapter_state.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_device.dart';

/// Contrato de acesso ao dongle OBD2 (interface no domain, impl no data).
///
/// É a *source of truth* da conexão para as camadas de UI: guarda a conexão
/// ativa e a stream de fases, e lança subclasses de `Failure` em erro.
abstract interface class DongleRepository {
  /// Estado do adaptador Bluetooth do telefone.
  Stream<BleAdapterState> get adapterState;

  /// Stream contínuo dos dongles vistos enquanto o scan está ativo.
  Stream<List<BleDevice>> scan({Duration timeout});

  /// Para o scan em andamento.
  Future<void> stopScan();

  /// Inicia a conexão com [deviceId] (retorna rápido). Acompanhe o progresso
  /// por [connectionPhase].
  Future<void> connect(String deviceId);

  /// Stream das fases da conexão atual (re-emitida pelo repository).
  Stream<BleConnectionPhase> get connectionPhase;

  /// Conexão ativa — exposta para a Fase 2 obter `write`/`incoming`. Nula
  /// quando não há conexão.
  BleConnection? get connection;

  /// `true` quando há uma conexão pronta (`ready`).
  bool get isReady;

  /// Encerra a conexão ativa (se houver).
  Future<void> disconnect();
}
