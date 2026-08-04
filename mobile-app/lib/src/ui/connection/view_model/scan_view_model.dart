import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:tccelta_mobile/src/core/errors/failure.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_adapter_state.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_device.dart';
import 'package:tccelta_mobile/src/domain/repositories/dongle_repository.dart';
import 'package:tccelta_mobile/src/ui/connection/connection_providers.dart';

/// Estado observável da tela de busca.
class ScanState {
  /// Cria o estado da busca.
  const ScanState({
    this.isScanning = false,
    this.devices = const [],
    this.adapterOn = true,
    this.failure,
  });

  /// Se o scan está ativo (mostra a barra de progresso/radar).
  final bool isScanning;

  /// Dongles encontrados até agora.
  final List<BleDevice> devices;

  /// Se o adaptador Bluetooth está ligado.
  final bool adapterOn;

  /// Falha corrente (nula quando não há erro).
  final Failure? failure;

  /// Cópia com campos sobrescritos. Use [clearFailure] para zerar a falha.
  ScanState copyWith({
    bool? isScanning,
    List<BleDevice>? devices,
    bool? adapterOn,
    Failure? failure,
    bool clearFailure = false,
  }) =>
      ScanState(
        isScanning: isScanning ?? this.isScanning,
        devices: devices ?? this.devices,
        adapterOn: adapterOn ?? this.adapterOn,
        failure: clearFailure ? null : (failure ?? this.failure),
      );
}

/// ViewModel da busca: dispara/para o scan e acompanha o adaptador. Toda a
/// lógica vive aqui; a tela só observa e renderiza.
class ScanViewModel extends Notifier<ScanState> {
  late final DongleRepository _repo;
  StreamSubscription<List<BleDevice>>? _scanSub;
  StreamSubscription<BleAdapterState>? _adapterSub;

  /// Fila que serializa as operações de scan. Toques rápidos em "Procurar
  /// novamente" disparam vários [startScan]/[stopScan] concorrentes; sem
  /// serializar, eles se intercalam (o segundo sobrescreve `_scanSub`, deixando
  /// o scan anterior órfão e ATIVO) e o dongle não é mais encontrado. A fila
  /// garante que uma operação termine antes de a próxima começar.
  Future<void> _scanQueue = Future<void>.value();

  /// Encadeia [action] após a operação de scan em andamento. Uma falha não
  /// envenena a fila (as próximas ainda rodam).
  Future<void> _enqueue(Future<void> Function() action) {
    final result = _scanQueue.then((_) => action());
    _scanQueue = result.then((_) {}, onError: (_) {});
    return result;
  }

  @override
  ScanState build() {
    _repo = ref.read(dongleRepositoryProvider);
    _adapterSub = _repo.adapterState.listen(
      (s) => state = state.copyWith(adapterOn: s == BleAdapterState.on),
    );
    ref.onDispose(() {
      unawaited(_scanSub?.cancel());
      unawaited(_adapterSub?.cancel());
    });
    return const ScanState();
  }

  /// Inicia (ou reinicia) a busca.
  ///
  /// Serializado via [_enqueue] para não intercalar chamadas concorrentes. Se
  /// já há um scan em andamento, a chamada é IGNORADA (coalescida): reiniciar o
  /// scan a cada toque em rajada dispara o throttle de scan do Android
  /// (~5 scans/30s), que trava os resultados e faz o dongle "sumir". Um novo
  /// scan só começa quando o anterior terminou (timeout) ou foi parado.
  Future<void> startScan() => _enqueue(() async {
        if (state.isScanning) return;
        await _scanSub?.cancel();
        _scanSub = null;
        state = state.copyWith(
          isScanning: true,
          devices: const [],
          clearFailure: true,
        );
        _scanSub = _repo.scan().listen(
          // Sem nome nem id (HEX) não dá pra rotular o dongle — omite.
          (devices) => state = state.copyWith(
            devices: devices
                .where((d) => d.displayName.isNotEmpty)
                .toList(growable: false),
          ),
          onError: (Object e) => state = state.copyWith(
            isScanning: false,
            failure: e is Failure ? e : null,
          ),
          onDone: () => state = state.copyWith(isScanning: false),
        );
      });

  /// Para a busca (ex.: ao selecionar um dongle). Também serializado, para não
  /// intercalar com um [startScan] em andamento.
  Future<void> stopScan() => _enqueue(() async {
        await _repo.stopScan();
        await _scanSub?.cancel();
        _scanSub = null;
        state = state.copyWith(isScanning: false);
      });
}

/// Provider do [ScanViewModel].
final NotifierProvider<ScanViewModel, ScanState> scanViewModelProvider =
    NotifierProvider<ScanViewModel, ScanState>(ScanViewModel.new);
