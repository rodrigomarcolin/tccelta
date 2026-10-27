import 'dart:async';
import 'dart:io' show Platform;

import 'package:flutter_blue_plus/flutter_blue_plus.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_adapter_state.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_bond_state.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_device.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_link_error.dart';
import 'package:tccelta_mobile/src/infra/ble/ble_link_error_classifier.dart';
import 'package:tccelta_mobile/src/services/ble/ble_service.dart';

/// Adapter concreto sobre `flutter_blue_plus` — o ÚNICO arquivo do app que
/// importa a lib. Converte os tipos da lib para os modelos de domínio.
class FlutterBluePlusBleService implements BleService {
  /// Cache de dispositivos vistos no scan, para (re)conectar por id.
  final Map<String, BluetoothDevice> _seen = {};

  @override
  Stream<BleAdapterState> get adapterState =>
      FlutterBluePlus.adapterState.map(_mapAdapter);

  @override
  Stream<List<BleDevice>> scan({
    List<String> withServiceUuids = const [],
    List<String> withNames = const [],
    List<String> withRemoteIds = const [],
    Duration timeout = const Duration(seconds: 15),
    bool continuousUpdates = false,
  }) {
    // Controller próprio para: (a) surfacar erros do startScan na stream,
    // (b) parar o scan automaticamente quando ninguém mais escuta e (c) FECHAR
    // a stream quando o scan termina (timeout/stop). O `scanResults` do FBP
    // nunca completa por conta própria, então sem (c) o `onDone` a montante
    // nunca dispararia e o estado ficaria "escaneando" para sempre.
    final controller = StreamController<List<BleDevice>>();
    StreamSubscription<List<ScanResult>>? sub;
    StreamSubscription<bool>? scanningSub;
    var wasScanning = false;

    controller
      ..onListen = () async {
        sub = FlutterBluePlus.onScanResults.listen((results) {
          for (final r in results) {
            _seen[r.device.remoteId.str] = r.device;
          }
          if (!controller.isClosed) {
            controller.add(results.map(_toDevice).toList());
          }
        }, onError: controller.addError);
        // Espelha o ciclo do scan: quando o FBP passa de "escaneando" para
        // "parado" (timeout), encerramos a stream (-> onDone a montante).
        scanningSub = FlutterBluePlus.isScanning.listen((scanning) {
          if (scanning) {
            wasScanning = true;
          } else if (wasScanning && !controller.isClosed) {
            unawaited(controller.close());
          }
        });
        try {
          await FlutterBluePlus.startScan(
            withServices: withServiceUuids.map(Guid.new).toList(),
            withNames: withNames,
            withRemoteIds: withRemoteIds,
            timeout: timeout,
            // Em modo contínuo processa todo anúncio (divisor 1, o default)
            // para o RSSI atualizar em tempo real durante o scan.
            continuousUpdates: continuousUpdates,
          );
        } on Object catch (e, st) {
          if (!controller.isClosed) controller.addError(e, st);
        }
      }
      ..onCancel = () async {
        await sub?.cancel();
        await scanningSub?.cancel();
        await FlutterBluePlus.stopScan();
      };
    return controller.stream;
  }

  @override
  Future<void> stopScan() => FlutterBluePlus.stopScan();

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
    final device = _seen[deviceId] ?? BluetoothDevice.fromId(deviceId);
    final conn = _FbpConnection(
      device: device,
      service: Guid(serviceUuid),
      rx: Guid(rxUuid),
      tx: Guid(txUuid),
      mtu: mtu,
      securingTimeout: securingTimeout,
      extraChannels: extraChannels,
    )..start(); // NÃO aguarda "ready"; começa a emitir fases imediatamente.
    return conn;
  }

  BleDevice _toDevice(ScanResult r) {
    final adv = r.advertisementData.advName;
    return BleDevice(
      id: r.device.remoteId.str,
      name: adv.isNotEmpty ? adv : r.device.platformName,
      rssi: r.rssi,
      manufacturerData: r.advertisementData.manufacturerData,
    );
  }

  @override
  bool get canManageBonds => Platform.isAndroid;

  @override
  Future<BleBondState> bondState(String deviceId) async {
    if (!Platform.isAndroid) return BleBondState.unknown;
    final state = await BluetoothDevice.fromId(deviceId).bondState.first;
    return switch (state) {
      BluetoothBondState.none => BleBondState.none,
      BluetoothBondState.bonding => BleBondState.bonding,
      BluetoothBondState.bonded => BleBondState.bonded,
    };
  }

  @override
  Future<void> removeBond(String deviceId) {
    if (!Platform.isAndroid) {
      throw UnsupportedError('removeBond só existe no Android');
    }
    return (_seen[deviceId] ?? BluetoothDevice.fromId(deviceId)).removeBond();
  }

  BleAdapterState _mapAdapter(BluetoothAdapterState s) => switch (s) {
    BluetoothAdapterState.on => BleAdapterState.on,
    BluetoothAdapterState.off => BleAdapterState.off,
    _ => BleAdapterState.unknown,
  };
}

/// Uma conexão BLE ativa sobre `flutter_blue_plus`, resiliente a reconexão.
class _FbpConnection implements BleConnection {
  _FbpConnection({
    required this.device,
    required this.service,
    required this.rx,
    required this.tx,
    required int mtu,
    required this.securingTimeout,
    List<BleChannelSpec> extraChannels = const [],
  }) : _requestedMtu = mtu,
       _channels = {
         // Os controllers nascem aqui: quem assina cedo não perde notificação
         // de nenhuma conexão.
         for (final spec in extraChannels)
           spec.serviceUuid.toUpperCase(): _FbpChannel(spec),
       };

  /// Tempo máximo que cada tentativa de connect (1ª conexão e reconexão)
  /// aguarda antes de desistir. Numa queda não intencional, é a janela de
  /// "grace period": durante ela o app fica em `reconnecting`/`connecting` e só
  /// emite `failed` se o dongle não voltar dentro desse prazo.
  static const Duration connectTimeout = Duration(seconds: 12);

  final BluetoothDevice device;
  final Guid service;
  final Guid rx;
  final Guid tx;
  final int _requestedMtu;

  /// Prazo do pareamento/cifra (inclui o tempo do usuário no diálogo do SO).
  final Duration securingTimeout;
  final Map<String, _FbpChannel> _channels;

  final StreamController<BleConnectionPhase> _phaseCtrl =
      StreamController<BleConnectionPhase>.broadcast();
  final StreamController<List<int>> _incomingCtrl =
      StreamController<List<int>>.broadcast();
  BleConnectionPhase _current = BleConnectionPhase.idle;
  BleConnectionPhase _stage = BleConnectionPhase.idle;
  BleLinkError? _lastError;

  BluetoothCharacteristic? _rx;
  BluetoothCharacteristic? _tx;
  StreamSubscription<List<int>>? _notifySub;
  StreamSubscription<BluetoothConnectionState>? _connStateSub;

  /// Só tratamos quedas como reconexão depois de já ter ficado `ready` uma vez
  /// (a stream de connectionState emite `disconnected` no valor inicial).
  bool _reachedReady = false;

  /// Encerramento intencional — impede reconexão após [disconnect].
  bool _closed = false;

  @override
  Stream<BleConnectionPhase> get phase => _phaseCtrl.stream;

  @override
  BleConnectionPhase get currentPhase => _current;

  @override
  BleLinkError? get lastError => _lastError;

  @override
  Stream<List<int>> get incoming => _incomingCtrl.stream;

  @override
  bool get isReady => _rx != null && _tx != null;

  @override
  BleChannel? channel(String serviceUuid) =>
      _channels[serviceUuid.toUpperCase()];

  @override
  int get mtu => device.mtuNow;

  void _emit(BleConnectionPhase p) {
    _current = p;
    if (!_phaseCtrl.isClosed) _phaseCtrl.add(p);
  }

  /// Muda a fase de transporte e guarda a etapa (para classificar falhas).
  void _stageTo(BleConnectionPhase p) {
    _stage = p;
    _emit(p);
  }

  /// Dispara o 1º connect e monitora quedas (o firmware volta a anunciar
  /// sozinho ao cair, então refazemos segurança + discover + notify).
  void start() {
    _connStateSub = device.connectionState.listen((state) async {
      if (state == BluetoothConnectionState.disconnected &&
          _reachedReady &&
          !_closed) {
        _rx = null;
        _tx = null;
        for (final c in _channels.values) {
          c.reset();
        }
        _reachedReady = false;
        _emit(BleConnectionPhase.reconnecting);
        await _reconnect();
      }
    });
    unawaited(
      _connectAndSetup().catchError((_) => _emit(BleConnectionPhase.failed)),
    );
  }

  /// Reconexão após queda. Erros de segurança (bond perdido, não autorizado,
  /// recusa) NÃO são repetidos, para não martelar o dongle; só `busy` (recusa
  /// transitória) tem novas tentativas, com espera crescente.
  Future<void> _reconnect() async {
    const delays = [
      Duration(seconds: 2),
      Duration(seconds: 4),
      Duration(seconds: 8),
    ];
    var attempt = 0;
    while (!_closed) {
      try {
        await _connectAndSetup();
        return;
      } on Object {
        if (_closed) return;
        if (_lastError == BleLinkError.busy && attempt < delays.length) {
          await Future<void>.delayed(delays[attempt++]);
          continue;
        }
        _emit(BleConnectionPhase.failed);
        return;
      }
    }
  }

  /// Traduz uma exceção da lib em [BleLinkError] (extrai os sinais e delega à
  /// função pura de classificação).
  BleLinkError _classify(Object error) {
    final isAndroid = Platform.isAndroid;
    final disconnectCode = device.disconnectReason?.code;
    var kind = LinkFailureKind.other;
    int? code;
    if (error is FlutterBluePlusException) {
      code = error.code;
      if (code == FbpErrorCode.createBondFailed.index) {
        kind = LinkFailureKind.createBondFailed;
      } else if (code == FbpErrorCode.timeout.index) {
        final pairing =
            error.function.toLowerCase().contains('bond') ||
            _stage == BleConnectionPhase.securing ||
            _stage == BleConnectionPhase.enablingNotify;
        kind = pairing
            ? LinkFailureKind.createBondTimeout
            : LinkFailureKind.connectTimeout;
      } else if (code == FbpErrorCode.deviceIsDisconnected.index ||
          code == FbpErrorCode.connectionCanceled.index) {
        kind = LinkFailureKind.disconnected;
      } else {
        kind = isAndroid
            ? LinkFailureKind.gattStatus
            : LinkFailureKind.coreBluetoothError;
      }
    }
    return classifyLinkError(
      isAndroid: isAndroid,
      stage: _stage,
      kind: kind,
      code: code,
      disconnectCode: disconnectCode,
    );
  }

  /// Idempotente: 1º connect e cada reconexão. Redescobrir + reabilitar notify
  /// após queda é OBRIGATÓRIO (características antigas ficam inválidas).
  Future<void> _connectAndSetup() async {
    _lastError = null;
    try {
      await _doConnectAndSetup();
    } on Object catch (e) {
      _lastError = _classify(e);
      try {
        await device.disconnect();
      } on Object {
        // Melhor esforço: o link pode já ter caído.
      }
      rethrow;
    }
  }

  Future<void> _doConnectAndSetup() async {
    _stageTo(BleConnectionPhase.connecting);
    // Projeto acadêmico sem fins lucrativos -> License.nonprofit (exigido pela
    // FlutterBluePlus 2.x). `mtu: null` desativa a negociação automática para
    // fazermos o requestMtu explícito (com a fase `optimizingLink`).
    await device.connect(
      license: License.nonprofit,
      timeout: connectTimeout,
      mtu: null,
    );

    // Enlace cifrado (LESC Just Works). No Android o app inicia o pareamento
    // com `createBond` logo após conectar (evita o diálogo duplicado); com
    // bond existente, o Security Request do dongle dispara a cifra. No iOS
    // não há API: o alerta do sistema aparece na 1ª operação que exige cifra
    // (a inscrição no TX, abaixo).
    _stageTo(BleConnectionPhase.securing);
    if (Platform.isAndroid) {
      final bond = await device.bondState.first;
      if (bond != BluetoothBondState.bonded) {
        await device.createBond(timeout: securingTimeout.inSeconds);
      }
      _stageTo(BleConnectionPhase.optimizingLink);
      try {
        await device.requestMtu(_requestedMtu);
      } on Object {
        // O MTU padrão do connect() já cobre; seguimos mesmo se falhar.
      }
    }

    _stageTo(BleConnectionPhase.discovering);
    final services = await device.discoverServices();
    final nus = services.firstWhere((s) => s.uuid == service);
    _rx = nus.characteristics.firstWhere((c) => c.uuid == rx);
    _tx = nus.characteristics.firstWhere((c) => c.uuid == tx);

    _stageTo(BleConnectionPhase.enablingNotify);
    await _notifySub?.cancel();
    // Assina ANTES de habilitar o notify: o dongle pode notificar dentro do
    // próprio `onSubscribe`, e `onValueReceived` é broadcast.
    _notifySub = _tx!.onValueReceived.listen(_incomingCtrl.add);
    device.cancelWhenDisconnected(_notifySub!);
    // A inscrição exige enlace cifrado: no iOS é aqui que o alerta de
    // pareamento aparece, então o prazo é o do pareamento.
    await _tx!.setNotifyValue(
      true,
      timeout: Platform.isIOS ? securingTimeout.inSeconds : 15,
    );

    // Canais extras: serviço ausente deixa o canal indisponível sem falhar a
    // conexão (firmware antigo segue só com o NUS).
    for (final c in _channels.values) {
      await c.setup(device, services);
    }

    _reachedReady = true;
    _emit(BleConnectionPhase.ready);
  }

  @override
  Future<void> write(List<int> bytes) async {
    final c = _rx;
    if (c == null) throw StateError('RX indisponível (link desconectado)');
    await c.write(bytes, withoutResponse: true); // RX aceita WRITE_NR.
  }

  @override
  Future<void> disconnect() async {
    _closed = true;
    await _connStateSub?.cancel();
    await _notifySub?.cancel();
    for (final c in _channels.values) {
      await c.dispose();
    }
    try {
      await device.disconnect();
    } finally {
      _emit(BleConnectionPhase.disconnected);
      await _phaseCtrl.close();
      await _incomingCtrl.close();
    }
  }
}

/// Canal GATT adicional sobre `flutter_blue_plus`. O controller de [incoming]
/// sobrevive às reconexões.
class _FbpChannel implements BleChannel {
  _FbpChannel(this.spec);

  final BleChannelSpec spec;
  final StreamController<List<int>> _incomingCtrl =
      StreamController<List<int>>.broadcast();

  BluetoothCharacteristic? _rx;
  BluetoothCharacteristic? _tx;
  StreamSubscription<List<int>>? _sub;

  @override
  Stream<List<int>> get incoming => _incomingCtrl.stream;

  @override
  bool get isReady => _rx != null && _tx != null;

  /// Procura o serviço em [services] e habilita o notify (assinando antes).
  Future<void> setup(
    BluetoothDevice device,
    List<BluetoothService> services,
  ) async {
    reset();
    final service = Guid(spec.serviceUuid);
    final matches = services.where((s) => s.uuid == service);
    if (matches.isEmpty) return;
    final chars = matches.first.characteristics;
    final rxChar = chars.where((c) => c.uuid == Guid(spec.rxUuid));
    final txChar = chars.where((c) => c.uuid == Guid(spec.txUuid));
    if (rxChar.isEmpty || txChar.isEmpty) return;
    final tx = txChar.first;
    await _sub?.cancel();
    _sub = tx.onValueReceived.listen(_incomingCtrl.add);
    device.cancelWhenDisconnected(_sub!);
    await tx.setNotifyValue(true);
    _rx = rxChar.first;
    _tx = tx;
  }

  /// Marca o canal como indisponível (link caiu).
  void reset() {
    _rx = null;
    _tx = null;
  }

  @override
  Future<void> write(List<int> bytes) async {
    final c = _rx;
    if (c == null) throw StateError('Canal indisponível (link desconectado)');
    await c.write(
      bytes,
      withoutResponse: !spec.writeWithResponse,
      allowLongWrite: spec.writeWithResponse,
    );
  }

  /// Libera o canal ao encerrar a conexão.
  Future<void> dispose() async {
    reset();
    await _sub?.cancel();
    await _incomingCtrl.close();
  }
}
