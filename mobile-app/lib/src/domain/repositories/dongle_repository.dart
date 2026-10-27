import 'package:tccelta_mobile/src/domain/ble/ble_adapter_state.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_bond_state.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_device.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_link_error.dart';
import 'package:tccelta_mobile/src/domain/ble/dongle_pairing_hint.dart';

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

  /// Se o SO deixa o app consultar, criar e remover bonds (Android).
  bool get canManageBonds;

  /// Estado do bond do SO com [deviceId] (`unknown` no iOS).
  Future<BleBondState> bondState(String deviceId);

  /// Remove o bond do SO. Lança `BleBondFailure` se o SO não remover e
  /// `BleBondUnsupportedFailure` no iOS.
  Future<void> removeBond(String deviceId);

  /// Dica de pareamento do anúncio do dongle; `null` = dongle não visto ou sem
  /// o campo.
  Stream<DonglePairingHint?> watchPairingHint(
    String deviceId, {
    Duration timeout,
  });

  /// Motivo da última falha da conexão ativa.
  BleLinkError? get lastLinkError;
}
