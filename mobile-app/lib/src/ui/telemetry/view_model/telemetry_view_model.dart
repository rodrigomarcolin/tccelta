import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:tccelta_mobile/src/core/errors/failure.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_adapter_info.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_reading.dart';
import 'package:tccelta_mobile/src/domain/repositories/obd2_repository.dart';
import 'package:tccelta_mobile/src/ui/telemetry/telemetry_providers.dart';

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
  /// no formato histórico ([HistoryPointsRange.max]). Alimenta o formato
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

/// ViewModel do painel: faz um ciclo de leitura sequencial de todos os PIDs e o
/// repete continuamente enquanto a tela estiver montada. Toda a lógica vive
/// aqui; a tela só observa e renderiza.
class TelemetryViewModel extends Notifier<TelemetryState> {
  late final Obd2Repository _repo;

  /// Sinaliza o encerramento do loop (a tela saiu / provider descartado).
  bool _stopped = false;

  /// Timer do próximo ciclo — cancelável no dispose (evita timer pendente).
  Timer? _timer;

  /// Intervalo entre ciclos de leitura. Um ciclo lê os PIDs um a um
  /// (respeitando o back-pressure); a pausa evita saturar a fila do firmware.
  static const Duration _interval = Duration(milliseconds: 800);

  @override
  TelemetryState build() {
    _repo = ref.read(obd2RepositoryProvider);
    ref.onDispose(() {
      _stopped = true;
      _timer?.cancel();
    });
    unawaited(_start());
    return const TelemetryState(isPolling: true);
  }

  /// Descobre os PIDs suportados uma vez (best-effort — se falhar, o painel cai
  /// no fallback de todos os curados) e então inicia o loop de leitura.
  Future<void> _start() async {
    try {
      final supported = await _repo.discoverSupported();
      if (_stopped) return;
      state = state.copyWith(supportedPids: supported);
    } on Object {
      // Descoberta é best-effort; segue para o polling mesmo assim.
    }
    await _cycle();
  }

  /// Um ciclo de leitura não-sobreposto: lê tudo, publica o estado e agenda o
  /// próximo ciclo por um [Timer] cancelável. Uma falha atualiza
  /// [TelemetryState.failure] mas NÃO interrompe o loop (o link pode voltar).
  Future<void> _cycle() async {
    if (_stopped) return;
    try {
      final readings = await _repo.readAll();
      if (_stopped) return;
      state = state.copyWith(
        readings: readings,
        adapterInfo: _repo.adapterInfo,
        clearFailure: true,
        history: _appendHistory(readings),
      );
    } on Failure catch (f) {
      if (_stopped) return;
      state = state.copyWith(failure: f);
    } on Object {
      // Erro inesperado: não derruba o loop.
    }
    if (_stopped) return;
    _timer = Timer(_interval, () => unawaited(_cycle()));
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
