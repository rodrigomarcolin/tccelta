import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:tccelta_mobile/src/core/errors/failure.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_adapter_info.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_reading.dart';
import 'package:tccelta_mobile/src/domain/repositories/obd2_repository.dart';
import 'package:tccelta_mobile/src/ui/telemetry/telemetry_providers.dart';
import 'package:tccelta_mobile/src/ui/telemetry/view_model/panel_view_model.dart';

/// Estado observável do painel de telemetria.
class TelemetryState {
  /// Cria o estado do painel.
  const TelemetryState({
    this.isPolling = false,
    this.readings = const [],
    this.supportedPids = const {},
    this.adapterInfo,
    this.failure,
    this.history = const {},
  });

  /// Se o ciclo de leitura está ativo.
  final bool isPolling;

  /// Última leva de leituras decodificadas (parcial é válida).
  final List<Obd2Reading> readings;

  /// PIDs suportados descobertos na conexão. Vazio até a descoberta rodar
  /// (nesse caso o painel mostra todos os PIDs curados como fallback).
  final Set<Obd2Pid> supportedPids;

  /// Identidade do adaptador conectado (versão + protocolo), para a top bar.
  final Obd2AdapterInfo? adapterInfo;

  /// Falha corrente (nula quando o último ciclo teve sucesso).
  final Failure? failure;

  /// Amostras recentes de cada PID, em ordem cronológica, limitadas a
  /// [Obd2ReadingHistory.maxSamples] por PID — o teto de qualquer indicador
  /// no formato histórico (`HistoryPointsRange.max`). Alimenta o formato
  /// "Histórico" do Painel; cada indicador recorta os últimos N pontos que
  /// sua própria customização pedir.
  final Map<Obd2Pid, List<double>> history;

  /// Cópia com campos sobrescritos. Use [clearFailure] para zerar a falha.
  TelemetryState copyWith({
    bool? isPolling,
    List<Obd2Reading>? readings,
    Set<Obd2Pid>? supportedPids,
    Obd2AdapterInfo? adapterInfo,
    Failure? failure,
    bool clearFailure = false,
    Map<Obd2Pid, List<double>>? history,
  }) => TelemetryState(
    isPolling: isPolling ?? this.isPolling,
    readings: readings ?? this.readings,
    supportedPids: supportedPids ?? this.supportedPids,
    adapterInfo: adapterInfo ?? this.adapterInfo,
    failure: clearFailure ? null : (failure ?? this.failure),
    history: history ?? this.history,
  );
}

/// Limite de amostras mantidas em [TelemetryState.history] por PID.
abstract final class Obd2ReadingHistory {
  /// Teto de amostras — o máximo que qualquer indicador em formato
  /// histórico pode pedir (`HistoryPointsRange.max`), para não crescer sem
  /// limite enquanto o painel fica aberto.
  static const int maxSamples = 50;
}

/// ViewModel do painel: enquanto a tela está montada, consome a leitura
/// agendada do repository ([Obd2Repository.watchReadings]), que lê cada PID
/// do Painel no período dele e os demais suportados a cada 5 s (catálogo da
/// busca de sensores). Mantém o foco do repository igual aos indicadores do
/// painel ativo. Toda a lógica vive aqui; a tela só observa e renderiza.
class TelemetryViewModel extends Notifier<TelemetryState> {
  late final Obd2Repository _repo;

  /// Sinaliza o encerramento (a tela saiu / provider descartado).
  bool _stopped = false;

  /// Assinatura da leitura agendada — cancelada no dispose.
  StreamSubscription<List<Obd2Reading>>? _readingsSub;

  @override
  TelemetryState build() {
    _repo = ref.read(obd2RepositoryProvider);
    ref
      ..onDispose(() {
        _stopped = true;
        unawaited(_readingsSub?.cancel());
      })
      ..listen<List<Obd2Pid>>(
        panelViewModelProvider.select((p) => p.value?.indicators ?? const []),
        (_, pids) => _repo.setFocus(pids),
      );
    unawaited(_start());
    return const TelemetryState(isPolling: true);
  }

  /// Descobre os PIDs suportados uma vez (best-effort — se falhar, o catálogo
  /// cai no fallback de todos os curados) e então assina a leitura agendada,
  /// já com o foco no painel atual.
  Future<void> _start() async {
    try {
      final supported = await _repo.discoverSupported();
      if (_stopped) return;
      state = state.copyWith(supportedPids: supported);
    } on Object {
      // Descoberta é best-effort; segue para a leitura mesmo assim.
    }
    if (_stopped) return;
    _repo.setFocus(
      ref.read(panelViewModelProvider).value?.indicators ?? const [],
    );
    _readingsSub = _repo.watchReadings().listen(
      (readings) {
        if (!_stopped) _applyReadings(readings);
      },
      onError: (Object e) {
        if (!_stopped && e is Failure) state = state.copyWith(failure: e);
      },
    );
  }

  /// Funde [readings] no estado por PID + ECU. Assim, duas ECUs que respondem
  /// ao mesmo PID não se sobrescrevem; telas legadas que indexam apenas por PID
  /// continuam escolhendo explicitamente uma leitura para exibição.
  void _applyReadings(List<Obd2Reading> readings) {
    final merged = {for (final r in state.readings) _readingKey(r): r};
    for (final r in readings) {
      merged[_readingKey(r)] = r;
    }
    state = state.copyWith(
      readings: merged.values.toList(growable: false),
      adapterInfo: _repo.adapterInfo,
      clearFailure: true,
      history: _appendHistory(readings),
    );
  }

  static String _readingKey(Obd2Reading reading) =>
      '${reading.pid.name}:${reading.ecuId?.toRadixString(16) ?? 'unknown'}';

  /// Acrescenta cada leitura de [readings] ao fim do histórico do seu PID,
  /// recortando para no máximo [Obd2ReadingHistory.maxSamples] amostras.
  Map<Obd2Pid, List<double>> _appendHistory(List<Obd2Reading> readings) {
    final history = {...state.history};
    for (final reading in readings) {
      final samples = <double>[
        ...history[reading.pid] ?? const <double>[],
        reading.value,
      ];
      history[reading.pid] = samples.length > Obd2ReadingHistory.maxSamples
          ? samples.sublist(samples.length - Obd2ReadingHistory.maxSamples)
          : samples;
    }
    return history;
  }
}

/// Provider do [TelemetryViewModel].
///
/// `autoDispose`: o polling deve parar ao sair do painel. Ao reentrar, o
/// `build()` roda de novo e reinicia o ciclo.
final NotifierProvider<TelemetryViewModel, TelemetryState>
telemetryViewModelProvider =
    NotifierProvider<TelemetryViewModel, TelemetryState>(
      TelemetryViewModel.new,
      isAutoDispose: true,
    );
