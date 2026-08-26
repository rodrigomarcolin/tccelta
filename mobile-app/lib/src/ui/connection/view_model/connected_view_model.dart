import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:tccelta_mobile/src/core/errors/failure.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_adapter_info.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';
import 'package:tccelta_mobile/src/domain/repositories/obd2_repository.dart';
import 'package:tccelta_mobile/src/ui/telemetry/telemetry_providers.dart';

/// Estado da sondagem do adaptador na tela "Conectado".
class ConnectedProbeState {
  /// Cria o estado da sondagem.
  const ConnectedProbeState({
    this.probing = true,
    this.info,
    this.supported = const {},
    this.failure,
  });

  /// Se a sondagem ainda está em andamento (mostra placeholders/carregando).
  final bool probing;

  /// Identidade do adaptador (versão + protocolo), quando disponível.
  final Obd2AdapterInfo? info;

  /// PIDs suportados descobertos (subconjunto conhecido).
  final Set<Obd2Pid> supported;

  /// Falha da sondagem (nula em sucesso).
  final Failure? failure;

  /// Cópia com campos sobrescritos.
  ConnectedProbeState copyWith({
    bool? probing,
    Obd2AdapterInfo? info,
    Set<Obd2Pid>? supported,
    Failure? failure,
    bool clearFailure = false,
  }) => ConnectedProbeState(
    probing: probing ?? this.probing,
    info: info ?? this.info,
    supported: supported ?? this.supported,
    failure: clearFailure ? null : (failure ?? this.failure),
  );
}

/// ViewModel da tela "Conectado": ao montar, sonda o adaptador uma vez
/// (init ELM327 + descoberta de capacidades, que também fixa o protocolo) e
/// expõe versão, protocolo e nº de sensores. É o "preparar adaptador" que o
/// hook da Fase 2 previa. A tela só observa e renderiza.
class ConnectedViewModel extends Notifier<ConnectedProbeState> {
  late final Obd2Repository _repo;
  bool _stopped = false;

  @override
  ConnectedProbeState build() {
    _repo = ref.read(obd2RepositoryProvider);
    ref.onDispose(() => _stopped = true);
    unawaited(_probe());
    return const ConnectedProbeState();
  }

  Future<void> _probe() async {
    try {
      final supported = await _repo.discoverSupported();
      if (_stopped) return;
      state = state.copyWith(
        probing: false,
        info: _repo.adapterInfo,
        supported: supported,
        clearFailure: true,
      );
    } on Failure catch (f) {
      if (_stopped) return;
      state = state.copyWith(probing: false, failure: f);
    } on Object {
      if (_stopped) return;
      state = state.copyWith(probing: false);
    }
  }
}

/// Provider do [ConnectedViewModel]. `autoDispose`: a sondagem é reexecutada
/// cada vez que a tela "Conectado" é exibida (o cache do repository torna a
/// reexecução barata).
final NotifierProvider<ConnectedViewModel, ConnectedProbeState>
connectedViewModelProvider =
    NotifierProvider<ConnectedViewModel, ConnectedProbeState>(
      ConnectedViewModel.new,
      isAutoDispose: true,
    );
