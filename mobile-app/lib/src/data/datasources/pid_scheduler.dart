import 'package:flutter/foundation.dart';
import 'package:tccelta_mobile/src/data/datasources/pid_schedule.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';

/// Espera sem PIDs configurados (firmware `kIdleWaitMs`).
const Duration kPidSchedulerIdleWait = Duration(seconds: 1);

/// O que o [PidScheduler] manda fazer agora.
sealed class PidDecision {
  const PidDecision();
}

/// Ler [pids] agora, na ordem.
@immutable
final class PidReadDecision extends PidDecision {
  /// Cria a decisão de leitura.
  const PidReadDecision({
    required this.pids,
    this.early = false,
    this.late = Duration.zero,
    this.slack = Duration.zero,
  });

  /// PIDs a ler, do vencimento mais antigo para o mais novo.
  final List<Obd2Pid> pids;

  /// Leitura antecipada (regra 2).
  final bool early;

  /// Leitura vencida: atraso do primeiro PID em relação ao vencimento.
  final Duration late;

  /// Leitura antecipada: folga até o próximo vencimento.
  final Duration slack;
}

/// Nada a ler: esperar [wait] e perguntar de novo.
@immutable
final class PidWaitDecision extends PidDecision {
  /// Cria a decisão de espera.
  const PidWaitDecision(this.wait);

  /// Quanto esperar.
  final Duration wait;
}

/// Estado de uma linha, para depuração e testes.
@immutable
class PidScheduleRow {
  /// Cria a linha.
  const PidScheduleRow({
    required this.setting,
    required this.nextDue,
    required this.lastRead,
  });

  /// Configuração do PID.
  final PidSetting setting;

  /// Próximo vencimento.
  final Duration nextDue;

  /// Última leitura (com ou sem resposta).
  final Duration lastRead;
}

/// Escalonador por PID, porta do `collector::Scheduler` do firmware
/// (plano-coleta §5.3). Decide qual PID ler e quando:
///
/// 1. Há PIDs vencidos → lê o vencido há mais tempo (e outros vencidos, até
///    `maxPerRead`).
/// 2. Senão, se o próximo vencimento está a mais de `Tmax` → lê
///    antecipadamente o de vencimento mais próximo entre os que já cumpriram
///    `Pᵢᵐⁱⁿ`.
/// 3. Senão, espera.
///
/// Puro e síncrono: os instantes vêm de quem chama (relógio monotônico), então
/// não há volta de contador como no `uint32_t` do firmware.
class PidScheduler {
  final List<_Entry> _entries = [];
  Duration _tmax = kDefaultPidTmax;

  /// Número de PIDs na tabela.
  int get length => _entries.length;

  /// Carrega a tabela de [config].
  ///
  /// PIDs novos vencem imediatamente, como no firmware. **Diferente do
  /// firmware**, que recomeça a tabela inteira (lá a config só muda num
  /// reinício da coleta), um PID que já estava na tabela mantém a última
  /// leitura: o vencimento continua o mesmo se o período não mudou e passa a
  /// `lastRead + Pᵢ` se mudou. Assim, editar o Painel não faz todos os PIDs
  /// vencerem juntos.
  void apply(PidScheduleConfig config, Duration now) {
    final previous = {for (final e in _entries) e.setting.pid: e};
    _tmax = config.tmax;
    _entries
      ..clear()
      ..addAll([
        for (final s in config.settings) _carryOver(s, previous[s.pid], now),
      ]);
  }

  static _Entry _carryOver(PidSetting s, _Entry? old, Duration now) {
    if (old == null) {
      return _Entry(s, nextDue: now, lastRead: now - s.minInterval);
    }
    final nextDue = old.setting.period == s.period
        ? old.nextDue
        : old.lastRead + s.period;
    return _Entry(s, nextDue: nextDue, lastRead: old.lastRead);
  }

  /// Próxima decisão em [now], lendo no máximo [maxPerRead] PIDs de uma vez.
  PidDecision next(Duration now, {int maxPerRead = 1}) {
    if (_entries.isEmpty) return const PidWaitDecision(kPidSchedulerIdleWait);
    final max = maxPerRead < 1 ? 1 : maxPerRead;
    final picked = List<bool>.filled(_entries.length, false);

    // Regra 1: vencidos, do mais atrasado para o menos.
    final overdue = _pick(picked, max, (e) => now >= e.nextDue);
    if (overdue.isNotEmpty) {
      return PidReadDecision(
        pids: [for (final e in overdue) e.setting.pid],
        late: now - overdue.first.nextDue,
      );
    }

    var earliestDue = _entries.first.nextDue;
    for (final e in _entries.skip(1)) {
      if (e.nextDue < earliestDue) earliestDue = e.nextDue;
    }

    // Regra 2: antecipa só com folga maior que Tmax até o próximo vencimento,
    // para nunca atrasar uma leitura agendada.
    final untilDue = earliestDue - now;
    if (untilDue > _tmax) {
      final early = _pick(picked, max, (e) => _canReadEarly(e, now));
      if (early.isNotEmpty) {
        return PidReadDecision(
          pids: [for (final e in early) e.setting.pid],
          early: true,
          slack: untilDue,
        );
      }
    }

    // Regra 3: espera o próximo vencimento ou a próxima antecipação possível.
    var wait = untilDue;
    for (final e in _entries) {
      final eligibleIn = _remaining(now, e.lastRead + e.setting.minInterval);
      if (eligibleIn < wait && untilDue - eligibleIn > _tmax) wait = eligibleIn;
    }
    return PidWaitDecision(wait);
  }

  /// Reagenda os PIDs de [decision] lidos em [now] (o fim da leitura), com ou
  /// sem resposta.
  void onRead(PidReadDecision decision, Duration now) {
    for (final pid in decision.pids) {
      final e = _find(pid);
      if (e == null) continue; // a tabela mudou durante a leitura
      e.lastRead = now;
      if (decision.early) {
        e.nextDue = now + e.setting.period;
      } else {
        // Mantém a fase: PIDs de mesmo período continuam lidos em sequência.
        e.nextDue += e.setting.period;
        if (now >= e.nextDue) e.nextDue = now;
      }
    }
  }

  /// Estado de cada linha, na ordem da tabela.
  List<PidScheduleRow> get rows => [
    for (final e in _entries)
      PidScheduleRow(
        setting: e.setting,
        nextDue: e.nextDue,
        lastRead: e.lastRead,
      ),
  ];

  /// Próximo vencimento de [pid], ou `null` se ele não está na tabela.
  Duration? nextDue(Obd2Pid pid) => _find(pid)?.nextDue;

  bool _canReadEarly(_Entry e, Duration now) =>
      now >= e.lastRead + e.setting.minInterval;

  /// Acrescenta, em ordem de vencimento, até [max] entradas que passam em
  /// [eligible]. Empate fica com a primeira da tabela.
  List<_Entry> _pick(
    List<bool> picked,
    int max,
    bool Function(_Entry e) eligible,
  ) {
    final out = <_Entry>[];
    while (out.length < max) {
      int? best;
      for (var i = 0; i < _entries.length; i++) {
        final e = _entries[i];
        if (picked[i] || !eligible(e)) continue;
        if (best == null || e.nextDue < _entries[best].nextDue) best = i;
      }
      if (best == null) break;
      picked[best] = true;
      out.add(_entries[best]);
    }
    return out;
  }

  _Entry? _find(Obd2Pid pid) {
    for (final e in _entries) {
      if (e.setting.pid == pid) return e;
    }
    return null;
  }

  static Duration _remaining(Duration now, Duration target) =>
      now >= target ? Duration.zero : target - now;
}

class _Entry {
  _Entry(this.setting, {required this.nextDue, required this.lastRead});

  final PidSetting setting;
  Duration nextDue;
  Duration lastRead;
}
