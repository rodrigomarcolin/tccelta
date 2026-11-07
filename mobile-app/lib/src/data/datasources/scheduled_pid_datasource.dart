import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:tccelta_mobile/src/core/time/monotonic_clock.dart';
import 'package:tccelta_mobile/src/data/datasources/pid_schedule.dart';
import 'package:tccelta_mobile/src/data/datasources/pid_scheduler.dart';
import 'package:tccelta_mobile/src/domain/obd2/elm_response.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';

/// Leitura de um PID por requisição, o `IPidReader` do firmware.
// ignore: one_member_abstracts
abstract interface class PidReader {
  /// Lê [pid] e devolve as respostas das ECUs. Lista vazia quando o PID não
  /// respondeu (`NO DATA`, *timeout*, resposta malformada): o PID é
  /// reagendado normalmente.
  ///
  /// Lança só quando não dá para ler nada (sem conexão). O laço então
  /// repassa o erro e espera antes de tentar de novo, sem reagendar o PID.
  Future<List<ElmResponse>> read(Obd2Pid pid);
}

/// Resultado de uma leitura agendada.
@immutable
class PidSample {
  /// Cria a amostra.
  const PidSample({
    required this.pid,
    required this.responses,
    required this.readAt,
    this.early = false,
  });

  /// PID lido.
  final Obd2Pid pid;

  /// Respostas das ECUs (nunca vazia numa amostra emitida).
  final List<ElmResponse> responses;

  /// Fim da leitura, no relógio monotônico do laço.
  final Duration readAt;

  /// Veio de uma leitura antecipada.
  final bool early;
}

/// Laço da tarefa de Aquisição do firmware (`Collector::poll`) sobre um
/// [PidReader]: pergunta ao [PidScheduler] o que fazer, lê ou dorme, e
/// reagenda com o instante do fim da leitura.
///
/// Uso único: o laço começa na escuta de [samples] e para no cancelamento.
/// [reconfigure] troca a tabela a qualquer momento e acorda o laço.
class ScheduledPidDatasource {
  /// Cria o laço sobre [reader] com a tabela inicial [config].
  ScheduledPidDatasource({
    required this.reader,
    required PidScheduleConfig config,
    MonotonicClock? clock,
    PidScheduler? scheduler,
    this.maxPerRead = 1,
    this.unavailableBackoff = kPidSchedulerIdleWait,
  }) : _clock = clock ?? SystemMonotonicClock(),
       _scheduler = scheduler ?? PidScheduler() {
    _scheduler.apply(config, _clock.now());
    _ctrl = StreamController<PidSample>(
      onListen: _start,
      onCancel: _stop,
    );
  }

  /// Leitor dos PIDs.
  final PidReader reader;

  final MonotonicClock _clock;
  final PidScheduler _scheduler;

  /// PIDs por decisão. O app lê um PID por comando ELM327, como o
  /// `SequentialPidReader` do firmware.
  final int maxPerRead;

  /// Espera depois de um erro do [PidReader] (sem conexão).
  final Duration unavailableBackoff;

  late final StreamController<PidSample> _ctrl;
  final Completer<void> _done = Completer<void>();
  bool _running = false;
  Timer? _sleepTimer;
  Completer<void>? _wake;

  /// Amostras com resposta, na ordem de leitura. Erros do [PidReader] chegam
  /// como eventos de erro e não encerram a stream.
  Stream<PidSample> get samples => _ctrl.stream;

  /// Completa quando o laço para.
  Future<void> get done => _done.future;

  /// Escalonador usado (estado para depuração).
  PidScheduler get scheduler => _scheduler;

  /// Troca a tabela e acorda o laço para reavaliar já.
  void reconfigure(PidScheduleConfig config) {
    _scheduler.apply(config, _clock.now());
    _wakeUp();
  }

  void _start() {
    if (_running) return;
    _running = true;
    unawaited(_run());
  }

  Future<void> _stop() async {
    _running = false;
    _wakeUp();
    await _done.future;
  }

  Future<void> _run() async {
    try {
      while (_running) {
        switch (_scheduler.next(_clock.now(), maxPerRead: maxPerRead)) {
          case PidWaitDecision(:final wait):
            await _sleep(wait);
          case final PidReadDecision d:
            await _read(d);
        }
      }
    } finally {
      _sleepTimer?.cancel();
      if (!_done.isCompleted) _done.complete();
      unawaited(_ctrl.close());
    }
  }

  Future<void> _read(PidReadDecision d) async {
    final results = <(Obd2Pid, List<ElmResponse>)>[];
    try {
      for (final pid in d.pids) {
        results.add((pid, await reader.read(pid)));
        if (!_running) return;
      }
    } on Object catch (e, st) {
      if (!_running) return;
      _ctrl.addError(e, st);
      await _sleep(unavailableBackoff);
      return;
    }
    final now = _clock.now();
    _scheduler.onRead(d, now);
    for (final (pid, responses) in results) {
      if (responses.isEmpty) continue;
      _ctrl.add(
        PidSample(pid: pid, responses: responses, readAt: now, early: d.early),
      );
    }
  }

  Future<void> _sleep(Duration wait) {
    if (!_running) return Future<void>.value();
    final wake = _wake = Completer<void>();
    _sleepTimer = Timer(wait, _wakeUp);
    return wake.future;
  }

  void _wakeUp() {
    _sleepTimer?.cancel();
    _sleepTimer = null;
    final wake = _wake;
    _wake = null;
    if (wake != null && !wake.isCompleted) wake.complete();
  }
}
