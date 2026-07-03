import 'dart:async';
import 'dart:io' show Platform;

import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:flutter_blue_plus/flutter_blue_plus.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_adapter_state.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_device.dart';
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
        sub = FlutterBluePlus.onScanResults.listen(
          (results) {
            for (final r in results) {
              _seen[r.device.remoteId.str] = r.device;
            }
            if (!controller.isClosed) {
              controller.add(results.map(_toDevice).toList());
            }
          },
          onError: controller.addError,
        );
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
    int mtu = 512,
  }) async {
    final device = _seen[deviceId] ?? BluetoothDevice.fromId(deviceId);
    final conn = _FbpConnection(
      device: device,
      service: Guid(serviceUuid),
      rx: Guid(rxUuid),
      tx: Guid(txUuid),
      mtu: mtu,
    )..start(); // NÃO aguarda "ready"; começa a emitir fases imediatamente.
    return conn;
  }

  BleDevice _toDevice(ScanResult r) {
    final adv = r.advertisementData.advName;
    return BleDevice(
      id: r.device.remoteId.str,
      name: adv.isNotEmpty ? adv : r.device.platformName,
      rssi: r.rssi,
    );
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
    required this.mtu,
  });

  /// Tempo máximo que cada tentativa de connect (1ª conexão e reconexão)
  /// aguarda antes de desistir. Numa queda não intencional, é a janela de
  /// "grace period": durante ela o app fica em `reconnecting`/`connecting` e só
  /// emite `failed` se o dongle não voltar dentro desse prazo.
  static const Duration connectTimeout = Duration(seconds: 12);

  final BluetoothDevice device;
  final Guid service;
  final Guid rx;
  final Guid tx;
  final int mtu;

  final StreamController<BleConnectionPhase> _phaseCtrl =
      StreamController<BleConnectionPhase>.broadcast();
  final StreamController<List<int>> _incomingCtrl =
      StreamController<List<int>>.broadcast();
  BleConnectionPhase _current = BleConnectionPhase.idle;

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
  Stream<List<int>> get incoming => _incomingCtrl.stream;

  @override
  bool get isReady => _rx != null && _tx != null;

  void _emit(BleConnectionPhase p) {
    _current = p;
    if (!_phaseCtrl.isClosed) _phaseCtrl.add(p);
  }

  /// Dispara o 1º connect e monitora quedas (o firmware volta a anunciar
  /// sozinho ao cair, então refazemos discover + notify).
  void start() {
    _connStateSub = device.connectionState.listen((state) async {
      if (state == BluetoothConnectionState.disconnected &&
          _reachedReady &&
          !_closed) {
        _rx = null;
        _tx = null;
        _reachedReady = false;
        _emit(BleConnectionPhase.reconnecting);
        try {
          await _connectAndSetup();
        } on Object {
          _emit(BleConnectionPhase.failed);
        }
      }
    });
    unawaited(
      _connectAndSetup().catchError((_) => _emit(BleConnectionPhase.failed)),
    );
  }

  /// Idempotente: 1º connect e cada reconexão. Redescobrir + reabilitar notify
  /// após queda é OBRIGATÓRIO (características antigas ficam inválidas).
  Future<void> _connectAndSetup() async {
    _emit(BleConnectionPhase.connecting);
    // Projeto acadêmico sem fins lucrativos -> License.nonprofit (exigido pela
    // FlutterBluePlus 2.x). `mtu: null` desativa a negociação automática para
    // fazermos o requestMtu explícito (com a fase `optimizingLink`).
    await device.connect(
      license: License.nonprofit,
      timeout: connectTimeout,
      mtu: null,
    );

    if (!kIsWeb && Platform.isAndroid) {
      _emit(BleConnectionPhase.optimizingLink);
      try {
        await device.requestMtu(mtu); // iOS negocia sozinho e ignora.
      } on Object {
        // O MTU padrão do connect() já cobre; seguimos mesmo se falhar.
      }
    }

    _emit(BleConnectionPhase.discovering);
    final services = await device.discoverServices();
    final nus = services.firstWhere((s) => s.uuid == service);
    _rx = nus.characteristics.firstWhere((c) => c.uuid == rx);
    _tx = nus.characteristics.firstWhere((c) => c.uuid == tx);

    _emit(BleConnectionPhase.enablingNotify);
    await _notifySub?.cancel();
    await _tx!.setNotifyValue(true);
    _notifySub = _tx!.onValueReceived.listen(_incomingCtrl.add);
    device.cancelWhenDisconnected(_notifySub!);

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
    try {
      await device.disconnect();
    } finally {
      _emit(BleConnectionPhase.disconnected);
      await _phaseCtrl.close();
      await _incomingCtrl.close();
    }
  }
}
