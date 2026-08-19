import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:tccelta_mobile/src/core/errors/failure.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';
import 'package:tccelta_mobile/src/domain/repositories/dongle_repository.dart';
import 'package:tccelta_mobile/src/domain/repositories/obd2_repository.dart';
import 'package:tccelta_mobile/src/ui/connection/connection_providers.dart';
import 'package:tccelta_mobile/src/ui/telemetry/telemetry_providers.dart';

/// Sub-fase da preparação OBD-II (Fase 2), que roda DEPOIS de o BLE ficar
/// `ready`: init do ELM327 e leitura das capacidades (PIDs suportados).
enum ConnectingPrep {
  /// Ainda não começou (BLE ainda não está `ready`).
  idle,

  /// Init do ELM327 em andamento (reset + echo off).
  preparing,

  /// Lendo as capacidades do veículo (bitmask de PIDs suportados).
  reading,

  /// Preparação concluída — pode seguir para "Conectado" (com os dados já
  /// preenchidos). Terminal também em caso de falha best-effort.
  done,
}

/// Estado do handshake: a fase BLE (Fase 1) + a sub-fase de preparação OBD-II
/// (Fase 2). Imutável, estilo `ScanState`.
class ConnectingState {
  /// Cria o estado com a [phase] BLE e a [prep] OBD-II.
  const ConnectingState({
    this.phase = BleConnectionPhase.idle,
    this.prep = ConnectingPrep.idle,
  });

  /// Fase do transporte BLE.
  final BleConnectionPhase phase;

  /// Sub-fase da preparação OBD-II (só avança após `phase == ready`).
  final ConnectingPrep prep;

  /// Cópia com campos sobrescritos.
  ConnectingState copyWith({BleConnectionPhase? phase, ConnectingPrep? prep}) =>
      ConnectingState(
        phase: phase ?? this.phase,
        prep: prep ?? this.prep,
      );
}

/// ViewModel do handshake de conexão. Reflete as fases BLE (Fase 1) e, ao ficar
/// `ready`, encadeia a preparação OBD-II (Fase 2): init do ELM327 + descoberta
/// de capacidades — para que a tela "Conectado" já receba protocolo e sensores
/// preenchidos. Só sinaliza "pronto para seguir" quando `prep == done`.
class ConnectingViewModel extends Notifier<ConnectingState> {
  late final DongleRepository _repo;
  late final Obd2Repository _obd;
  StreamSubscription<BleConnectionPhase>? _sub;

  @override
  ConnectingState build() {
    _repo = ref.read(dongleRepositoryProvider);
    _obd = ref.read(obd2RepositoryProvider);
    ref.onDispose(() => _sub?.cancel());
    return const ConnectingState();
  }

  /// Inicia a conexão com [deviceId] e passa a refletir as fases do repository.
  ///
  /// Reinicia o estado do zero (fase + preparação): este ViewModel é mantido
  /// vivo globalmente (o `ConnectionGuard` o observa), então numa RECONEXÃO
  /// após "Conexão perdida" o `prep` ainda estaria em `done` da sessão anterior
  /// — os dois últimos passos apareceriam já concluídos e a preparação não
  /// reexecutaria. Zerar aqui garante que tudo recomece.
  Future<void> connect(String deviceId) async {
    state = const ConnectingState(phase: BleConnectionPhase.connecting);
    await _sub?.cancel();
    _sub = _repo.connectionPhase.listen(
      (p) {
        state = state.copyWith(phase: p);
        if (p == BleConnectionPhase.ready &&
            state.prep == ConnectingPrep.idle) {
          unawaited(_prepare());
        }
      },
      onError: (_) => state = state.copyWith(phase: BleConnectionPhase.failed),
    );
    try {
      await _repo.connect(deviceId);
    } on Failure {
      state = state.copyWith(phase: BleConnectionPhase.failed);
    }
  }

  /// Preparação OBD-II sobre a conexão já `ready`: init do ELM327 e leitura das
  /// capacidades. Best-effort — se qualquer passo falhar, ainda conclui
  /// (`done`) e as telas seguintes degradam graciosamente (protocolo `—`, 0
  /// sensores); o link BLE está saudável, então não é "Conexão perdida".
  Future<void> _prepare() async {
    state = state.copyWith(prep: ConnectingPrep.preparing);
    try {
      await _obd.initialize();
      state = state.copyWith(prep: ConnectingPrep.reading);
      await _obd.discoverSupported();
    } on Object {
      // Best-effort: segue mesmo assim.
    }
    state = state.copyWith(prep: ConnectingPrep.done);
  }

  /// Cancela/encerra a conexão em andamento.
  Future<void> cancel() => _repo.disconnect();
}

/// Provider do [ConnectingViewModel].
final NotifierProvider<ConnectingViewModel, ConnectingState>
connectingViewModelProvider =
    NotifierProvider<ConnectingViewModel, ConnectingState>(
      ConnectingViewModel.new,
    );
