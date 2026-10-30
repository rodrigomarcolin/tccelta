import 'dart:async';

import 'package:tccelta_mobile/src/domain/ble/ble_adapter_state.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_bond_state.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_device.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_link_error.dart';
import 'package:tccelta_mobile/src/domain/ble/dongle_pairing_hint.dart';
import 'package:tccelta_mobile/src/services/ble/ble_service.dart';

/// [BleService] falso para exercitar as camadas sem hardware BLE.
///
/// Como só o adapter conhece a lib real, trocar apenas `bleServiceProvider` por
/// este fake permite rodar telas e view models em testes/dev.
class FakeBleService implements BleService {
  /// Cria o fake com os [devices], estados de adaptador e sequência de fases.
  FakeBleService({
    this.devices = const [],
    this.deviceFrames = const [],
    this.adapterStates = const [BleAdapterState.on],
    this.scanError,
    this.holdScanOpen = false,
    this.channels = const {},
    this.onPhase,
    this.canManageBonds = true,
    this.bondStates = const {},
    this.removeBondError,
    this.hintFrames = const [],
    this.plans = const [],
    this.phases = const [
      BleConnectionPhase.connecting,
      BleConnectionPhase.securing,
      BleConnectionPhase.optimizingLink,
      BleConnectionPhase.discovering,
      BleConnectionPhase.enablingNotify,
      BleConnectionPhase.ready,
    ],
  });

  /// Dongles emitidos pelo scan (emissão única).
  final List<BleDevice> devices;

  /// Sequência de emissões do scan para simular resultados mudando ao vivo
  /// (ex.: mesmo dongle com RSSI variando). Quando não vazio, tem precedência
  /// sobre [devices]: cada quadro é emitido em um turno do loop de eventos.
  final List<List<BleDevice>> deviceFrames;

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

  /// Canais extras (por UUID do serviço) expostos pela conexão.
  final Map<String, BleChannel> channels;

  /// Chamado a cada fase, ANTES de ela ser emitida (permite simular uma
  /// notificação enviada durante o `setNotifyValue`).
  final void Function(BleConnectionPhase phase)? onPhase;

  /// Se o fake simula um SO que deixa o app gerenciar bonds (Android).
  @override
  final bool canManageBonds;

  /// Estado de bond por id de dispositivo (padrão `none`).
  final Map<String, BleBondState> bondStates;

  /// Ids cujo bond foi removido por [removeBond], em ordem.
  final List<String> removeBondCalls = [];

  /// Se não nulo, [removeBond] lança este erro.
  final Object? removeBondError;

  /// Quadros do scan filtrado por id (anúncios do dongle com dados de
  /// fabricante): cada quadro é emitido espaçado, com o scan aberto.
  final List<List<BleDevice>> hintFrames;

  /// Planos de conexão consumidos em ordem; sem plano, vale [phases].
  final List<FakeConnectPlan> plans;

  /// Quantas conexões foram abertas.
  int connectCount = 0;

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
    List<String> withRemoteIds = const [],
    Duration timeout = const Duration(seconds: 15),
    bool continuousUpdates = false,
  }) {
    scanCount++;
    if (withRemoteIds.isNotEmpty) return _hintScan();
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
        if (_scanActive) {
          controller.add(const []);
        } else if (deviceFrames.isNotEmpty) {
          // Emite o 1º quadro já; os seguintes (RSSI variando ao vivo) são
          // espaçados por delay real para os widget tests conseguirem pumpar
          // entre eles (sem isso, todos os Duration.zero coalesceriam).
          controller.add(deviceFrames.first);
          for (var i = 1; i < deviceFrames.length; i++) {
            final frame = deviceFrames[i];
            unawaited(
              Future<void>.delayed(Duration(milliseconds: 50 * i), () {
                if (!controller.isClosed) controller.add(frame);
              }),
            );
          }
        } else {
          controller.add(devices);
        }
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

  Stream<List<BleDevice>> _hintScan() {
    final controller = StreamController<List<BleDevice>>();
    controller.onListen = () {
      for (var i = 0; i < hintFrames.length; i++) {
        final frame = hintFrames[i];
        unawaited(
          Future<void>.delayed(Duration(milliseconds: 20 * (i + 1)), () {
            if (!controller.isClosed) controller.add(frame);
          }),
        );
      }
    };
    return controller.stream;
  }

  @override
  Future<BleBondState> bondState(String deviceId) async => canManageBonds
      ? (bondStates[deviceId] ?? BleBondState.none)
      : BleBondState.unknown;

  @override
  Future<void> removeBond(String deviceId) async {
    if (!canManageBonds) throw UnsupportedError('iOS');
    if (removeBondError != null) {
      throw Exception(removeBondError);
    }
    removeBondCalls.add(deviceId);
    bondStates[deviceId] = BleBondState.none;
  }

  @override
  Future<BleConnection> connect({
    required String deviceId,
    required String serviceUuid,
    required String rxUuid,
    required String txUuid,
    List<BleChannelSpec> extraChannels = const [],
    int mtu = 512,
    Duration securingTimeout = const Duration(seconds: 30),
  }) async {
    connectCount++;
    final plan = plans.length >= connectCount ? plans[connectCount - 1] : null;
    return FakeBleConnection(
      plan?.phases ?? phases,
      lastError: plan?.error,
      channelsByService: channels,
      onPhase: onPhase,
    );
  }
}

/// Conexão falsa que percorre uma sequência de fases, uma por turno do loop de
/// eventos (para que os assinantes tenham tempo de se registrar).
class FakeBleConnection implements BleConnection {
  /// Cria a conexão e começa a emitir a sequência de fases recebida.
  FakeBleConnection(
    this._phases, {
    this.lastError,
    this.channelsByService = const {},
    this.onPhase,
  }) {
    unawaited(_run());
  }

  final List<BleConnectionPhase> _phases;

  /// Motivo da falha simulada.
  @override
  final BleLinkError? lastError;

  /// Canais extras por UUID do serviço.
  final Map<String, BleChannel> channelsByService;

  /// Chamado antes de cada fase ser emitida.
  final void Function(BleConnectionPhase phase)? onPhase;
  final StreamController<BleConnectionPhase> _phaseCtrl =
      StreamController<BleConnectionPhase>.broadcast();
  final StreamController<List<int>> _incomingCtrl =
      StreamController<List<int>>.broadcast();
  BleConnectionPhase _current = BleConnectionPhase.idle;

  Future<void> _run() async {
    for (final p in _phases) {
      await Future<void>.delayed(Duration.zero);
      onPhase?.call(p);
      _current = p;
      if (!_phaseCtrl.isClosed) _phaseCtrl.add(p);
    }
  }

  /// Emite [phase] manualmente (simula queda e reconexão).
  void emitPhase(BleConnectionPhase phase) {
    _current = phase;
    if (!_phaseCtrl.isClosed) _phaseCtrl.add(phase);
  }

  @override
  BleChannel? channel(String serviceUuid) => channelsByService[serviceUuid];

  @override
  int get mtu => 247;

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

/// Plano de uma conexão do [FakeBleService]: fases percorridas e o motivo da
/// falha (quando termina em `failed`).
class FakeConnectPlan {
  /// Cria o plano.
  const FakeConnectPlan(this.phases, {this.error});

  /// Fases emitidas.
  final List<BleConnectionPhase> phases;

  /// `BleConnection.lastError` da conexão.
  final BleLinkError? error;
}

/// Dados de fabricante do anúncio do dongle (`FF FF 01 flags`).
Map<int, List<int>> manufacturerFor({
  required bool windowOpen,
  bool hasBond = false,
}) => {
  DonglePairingHint.companyId: [
    DonglePairingHint.formatVersion,
    (windowOpen ? 0x01 : 0) | (hasBond ? 0x02 : 0),
  ],
};
