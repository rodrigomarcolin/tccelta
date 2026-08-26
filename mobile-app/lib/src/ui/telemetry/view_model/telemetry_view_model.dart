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

/// ViewModel do painel: mantém dois ciclos de leitura independentes e cada um
/// não-sobreposto (só agenda a própria próxima volta depois que a anterior
/// termina) enquanto a tela estiver montada — um ciclo **rápido** (800ms) que
/// lê só os indicadores que o usuário tem no Painel agora, para essas leituras
/// ficarem responsivas, e um ciclo **completo** (5s) que varre todos os PIDs
/// suportados, para descobrir/atualizar sensores mesmo quando não estão no
/// Painel (ex.: a tela de busca de sensores). Toda a lógica vive aqui; a tela
/// só observa e renderiza.
class TelemetryViewModel extends Notifier<TelemetryState> {
  late final Obd2Repository _repo;

  /// Sinaliza o encerramento dos loops (a tela saiu / provider descartado).
  bool _stopped = false;

  /// Timer do próximo ciclo rápido — cancelável no dispose.
  Timer? _fastTimer;

  /// Timer do próximo ciclo completo — cancelável no dispose.
  Timer? _fullTimer;

  /// Intervalo do ciclo rápido: só os indicadores do Painel.
  static const Duration _fastInterval = Duration(milliseconds: 800);

  /// Intervalo do ciclo completo: todos os PIDs suportados.
  static const Duration _fullInterval = Duration(seconds: 5);

  @override
  TelemetryState build() {
    _repo = ref.read(obd2RepositoryProvider);
    ref.onDispose(() {
      _stopped = true;
      _fastTimer?.cancel();
      _fullTimer?.cancel();
    });
    unawaited(_start());
    return const TelemetryState(isPolling: true);
  }

  /// Descobre os PIDs suportados uma vez (best-effort — se falhar, o ciclo
  /// completo cai no fallback de todos os curados) e então inicia os dois
  /// loops de leitura.
  Future<void> _start() async {
    try {
      final supported = await _repo.discoverSupported();
      if (_stopped) return;
      state = state.copyWith(supportedPids: supported);
    } on Object {
      // Descoberta é best-effort; segue para o polling mesmo assim.
    }
    unawaited(_fastCycle());
    unawaited(_fullCycle());
  }

  /// Ciclo rápido: lê só os PIDs que o usuário tem no Painel neste exato
  /// momento — consulta o [panelViewModelProvider] a cada volta (não uma vez
  /// no `build()`), para sempre refletir add/remove sem precisar reiniciar o
  /// loop. Painel vazio pula a leitura mas mantém o cadenciamento.
  Future<void> _fastCycle() async {
    if (_stopped) return;
    final panelPids = ref.read(panelViewModelProvider).indicators;
    if (panelPids.isEmpty) {
      _fastTimer = Timer(_fastInterval, () => unawaited(_fastCycle()));
      return;
    }
    try {
      final readings = await _repo.readMany(panelPids);
      if (_stopped) return;
      _applyReadings(readings);
    } on Failure catch (f) {
      if (_stopped) return;
      state = state.copyWith(failure: f);
    } on Object {
      // Erro inesperado: não derruba o loop.
    }
    if (_stopped) return;
    _fastTimer = Timer(_fastInterval, () => unawaited(_fastCycle()));
  }

  /// Ciclo completo: varre todos os PIDs suportados (ou curados, no fallback
  /// sem descoberta), independentemente do que está no Painel — mantém o
  /// catálogo de sensores atualizado.
  Future<void> _fullCycle() async {
    if (_stopped) return;
    try {
      final readings = await _repo.readAll();
      if (_stopped) return;
      _applyReadings(readings);
    } on Failure catch (f) {
      if (_stopped) return;
      state = state.copyWith(failure: f);
    } on Object {
      // Erro inesperado: não derruba o loop.
    }
    if (_stopped) return;
    _fullTimer = Timer(_fullInterval, () => unawaited(_fullCycle()));
  }

  /// Funde [readings] no estado: cada leitura sobrescreve a anterior do mesmo
  /// PID, preservando as leituras de PIDs não tocados por este ciclo (os dois
  /// ciclos — rápido e completo — reportam PIDs potencialmente diferentes a
  /// cada volta).
  void _applyReadings(List<Obd2Reading> readings) {
    final merged = {for (final r in state.readings) r.pid: r};
    for (final r in readings) {
      merged[r.pid] = r;
    }
    state = state.copyWith(
      readings: merged.values.toList(growable: false),
      adapterInfo: _repo.adapterInfo,
      clearFailure: true,
      history: _appendHistory(readings),
    );
  }

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
