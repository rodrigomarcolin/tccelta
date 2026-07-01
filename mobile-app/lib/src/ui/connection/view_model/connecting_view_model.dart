import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:tccelta_mobile/src/core/errors/failure.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';
import 'package:tccelta_mobile/src/domain/repositories/dongle_repository.dart';
import 'package:tccelta_mobile/src/ui/connection/connection_providers.dart';

/// ViewModel do handshake de conexão. O estado é a [BleConnectionPhase] atual.
///
/// Cobre só a Fase 1 (transporte BLE). Os passos de init do ELM327 e leitura
/// de capacidades são Fase 2 e continuam DEPOIS de `ready` (ver hook abaixo).
class ConnectingViewModel extends Notifier<BleConnectionPhase> {
  late final DongleRepository _repo;
  StreamSubscription<BleConnectionPhase>? _sub;

  @override
  BleConnectionPhase build() {
    _repo = ref.read(dongleRepositoryProvider);
    ref.onDispose(() => _sub?.cancel());
    return BleConnectionPhase.idle;
  }

  /// Inicia a conexão com [deviceId] e passa a refletir as fases do repository.
  Future<void> connect(String deviceId) async {
    state = BleConnectionPhase.connecting;
    await _sub?.cancel();
    _sub = _repo.connectionPhase.listen(
      (p) => state = p,
      onError: (_) => state = BleConnectionPhase.failed,
    );
    try {
      await _repo.connect(deviceId);
    } on Failure {
      state = BleConnectionPhase.failed;
    }
    // HOOK FASE 2: quando state == ready, o fluxo de "preparar adaptador"
    // continua com o ELM327 sobre a conexão ativa:
    //   final conn = _repo.connection!;
    //   final elm = Elm327(conn.write);
    //   conn.incoming.listen(elm.onBytes);
    //   await initElm(elm); // + ler capacidades (PIDs)
  }

  /// Cancela/encerra a conexão em andamento.
  Future<void> cancel() => _repo.disconnect();
}

/// Provider do [ConnectingViewModel].
final NotifierProvider<ConnectingViewModel, BleConnectionPhase>
    connectingViewModelProvider =
    NotifierProvider<ConnectingViewModel, BleConnectionPhase>(
  ConnectingViewModel.new,
);
