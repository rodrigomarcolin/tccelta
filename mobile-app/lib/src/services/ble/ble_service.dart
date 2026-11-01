import 'package:meta/meta.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_adapter_state.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_bond_state.dart';
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
  /// filtros (que são "OU" entre si). Emite uma nova lista a cada atualização
  /// de resultados.
  ///
  /// Com [continuousUpdates] `true`, processa também anúncios repetidos para
  /// manter o RSSI atualizado em tempo real (custa mais bateria/CPU); com
  /// `false` (default), cada dispositivo é reportado uma vez e o RSSI congela
  /// no 1º anúncio. [withRemoteIds] filtra por id de plataforma.
  Stream<List<BleDevice>> scan({
    List<String> withServiceUuids,
    List<String> withNames,
    List<String> withRemoteIds,
    Duration timeout,
    bool continuousUpdates,
  });

  /// Para o scan em andamento.
  Future<void> stopScan();

  /// Conecta e prepara a conexão (segurança + MTU + discover + notify) de forma
  /// resiliente a reconexão. Retorna o handle IMEDIATAMENTE; o progresso vem
  /// por [BleConnection.phase]. [securingTimeout] é o prazo do pareamento/cifra
  /// (inclui o tempo do usuário no diálogo do SO). Com [requireBond] `false`,
  /// a etapa de segurança é pulada: o app não cria bond e o link segue sem
  /// cifra, para dispositivos que não a exigem.
  Future<BleConnection> connect({
    required String deviceId,
    required String serviceUuid,
    required String rxUuid,
    required String txUuid,
    List<BleChannelSpec> extraChannels = const [],
    int mtu,
    Duration securingTimeout,
    bool requireBond,
  });

  /// Se o SO permite ao app consultar, criar e remover bonds (Android).
  bool get canManageBonds;

  /// Estado do bond do SO com [deviceId]. Sem suporte: [BleBondState.unknown].
  Future<BleBondState> bondState(String deviceId);

  /// Remove o bond do SO com [deviceId]. Sem suporte: lança [UnsupportedError].
  Future<void> removeBond(String deviceId);
}

/// Par RX/TX de um serviço GATT adicional, aberto na mesma conexão.
@immutable
class BleChannelSpec {
  /// Cria a especificação do canal.
  const BleChannelSpec({
    required this.serviceUuid,
    required this.rxUuid,
    required this.txUuid,
    this.writeWithResponse = true,
  });

  /// UUID do serviço GATT.
  final String serviceUuid;

  /// Característica de escrita (app → dispositivo).
  final String rxUuid;

  /// Característica de notificação (dispositivo → app).
  final String txUuid;

  /// Escrita com resposta (permite *long write*). @default true
  final bool writeWithResponse;
}
