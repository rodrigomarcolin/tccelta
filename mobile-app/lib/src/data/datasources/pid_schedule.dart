import 'package:flutter/foundation.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';

/// `Tmax` padrão: pior tempo de resposta esperado de um PID. Mesmo valor do
/// firmware (`kDefaultTmaxMs`, plano-coleta §2).
const Duration kDefaultPidTmax = Duration(milliseconds: 50);

/// Piso de `Pᵢ` e `Pᵢᵐⁱⁿ` (firmware `kMinPeriodMs`).
const Duration kMinPidPeriod = Duration(milliseconds: 100);

/// Período dos PIDs que só alimentam o catálogo de sensores (fora do Painel).
/// Substitui o antigo ciclo completo de 5 s do `TelemetryViewModel`.
const Duration kCatalogPidPeriod = Duration(seconds: 5);

/// Utilização máxima aceitável (`Umax`, plano-coleta §5.3).
const double kMaxPidUtilization = 0.8;

/// Linha da tabela de escalonamento: um PID e seus dois intervalos.
@immutable
class PidSetting {
  /// Cria a linha com os intervalos já validados (ver as fábricas).
  /// Linha com os dois intervalos explícitos, ambos com o piso de 100 ms.
  factory PidSetting(
    Obd2Pid pid, {
    required Duration period,
    required Duration minInterval,
  }) => PidSetting._(pid, _floor(period), _floor(minInterval));

  const PidSetting._(this.pid, this.period, this.minInterval);

  /// PID que lê antecipadamente: `Pᵢᵐⁱⁿ = max(Pᵢ/2, 100 ms)`, como o
  /// `defaultMinInterval` do firmware.
  factory PidSetting.early(Obd2Pid pid, Duration period) {
    final p = _floor(period);
    return PidSetting._(pid, p, _floor(p ~/ 2));
  }

  /// PID que muda devagar e nunca lê antecipadamente: `Pᵢᵐⁱⁿ = Pᵢ`.
  factory PidSetting.slow(Obd2Pid pid, Duration period) {
    final p = _floor(period);
    return PidSetting._(pid, p, p);
  }

  /// PID lido.
  final Obd2Pid pid;

  /// `Pᵢ`: intervalo máximo desejado entre leituras.
  final Duration period;

  /// `Pᵢᵐⁱⁿ`: intervalo mínimo para uma leitura antecipada.
  final Duration minInterval;

  static Duration _floor(Duration d) => d < kMinPidPeriod ? kMinPidPeriod : d;

  @override
  bool operator ==(Object other) =>
      other is PidSetting &&
      other.pid == pid &&
      other.period == period &&
      other.minInterval == minInterval;

  @override
  int get hashCode => Object.hash(pid, period, minInterval);

  @override
  String toString() =>
      'PidSetting(${pid.command}, P=${period.inMilliseconds} ms, '
      'Pmin=${minInterval.inMilliseconds} ms)';
}

/// Fator `kᵢ` de um PID (`Pᵢ = kᵢ·Tmax`) e se ele lê antecipadamente.
typedef PidFactor = ({int k, bool early});

/// Configuração do escalonador: `Tmax` e a tabela de PIDs, na ordem de
/// desempate (empates de vencimento vão para o primeiro da lista).
@immutable
class PidScheduleConfig {
  /// Cria a configuração. PIDs repetidos ficam só com a primeira linha.
  PidScheduleConfig({
    required List<PidSetting> settings,
    this.tmax = kDefaultPidTmax,
  }) : settings = List.unmodifiable(_dedupe(settings));

  /// Mesma tabela do firmware, com os 7 PIDs na ordem dele.
  factory PidScheduleConfig.firmwareDefaults({
    Duration tmax = kDefaultPidTmax,
  }) => PidScheduleConfig(
    tmax: tmax,
    settings: [
      for (final MapEntry(key: pid, value: f) in firmwareFactors.entries)
        _fromFactor(pid, f, tmax),
    ],
  );

  /// Tabela da telemetria do app: os PIDs de [focus] (o Painel) com o período
  /// do firmware e, depois deles, os de [catalog] que sobraram, lentos, a cada
  /// [kCatalogPidPeriod] e sem leitura antecipada.
  factory PidScheduleConfig.telemetry({
    required Iterable<Obd2Pid> focus,
    Iterable<Obd2Pid> catalog = const [],
    Duration tmax = kDefaultPidTmax,
  }) {
    final focused = focus.toSet();
    return PidScheduleConfig(
      tmax: tmax,
      settings: [
        for (final pid in focused)
          _fromFactor(pid, firmwareFactors[pid] ?? _defaultFactor, tmax),
        for (final pid in catalog)
          if (!focused.contains(pid)) PidSetting.slow(pid, kCatalogPidPeriod),
      ],
    );
  }

  /// `Tmax` usado na regra 2 e no cálculo de utilização.
  final Duration tmax;

  /// Linhas da tabela, sem PIDs repetidos.
  final List<PidSetting> settings;

  /// Tabela 10 do firmware (`CollectorConfig::defaults`). PIDs do Painel fora
  /// dela usam [_defaultFactor].
  static const Map<Obd2Pid, PidFactor> firmwareFactors = {
    Obd2Pid.rpm: (k: 5, early: true),
    Obd2Pid.speed: (k: 5, early: true),
    Obd2Pid.acceleratorPedalD: (k: 5, early: true),
    Obd2Pid.engineLoad: (k: 12, early: true),
    Obd2Pid.maf: (k: 12, early: true),
    Obd2Pid.coolantTemp: (k: 100, early: false),
    Obd2Pid.fuelTankLevel: (k: 200, early: false),
  };

  /// PID do Painel sem linha na Tabela 10: 1 s (com `Tmax` de 50 ms), com
  /// leitura antecipada.
  static const PidFactor _defaultFactor = (k: 20, early: true);

  /// Utilização `U = Σ Tmax/Pᵢ` (firmware `utilization`). Acima de
  /// [kMaxPidUtilization] o canal sequencial não cumpre todos os períodos.
  double get utilization => settings.fold(
    0,
    (sum, s) => sum + tmax.inMicroseconds / s.period.inMicroseconds,
  );

  /// `true` se [utilization] cabe em [kMaxPidUtilization].
  bool get isFeasible => utilization <= kMaxPidUtilization;

  static PidSetting _fromFactor(Obd2Pid pid, PidFactor f, Duration tmax) {
    final period = tmax * f.k;
    return f.early
        ? PidSetting.early(pid, period)
        : PidSetting.slow(pid, period);
  }

  static List<PidSetting> _dedupe(List<PidSetting> settings) {
    final seen = <Obd2Pid>{};
    return [
      for (final s in settings)
        if (seen.add(s.pid)) s,
    ];
  }
}
