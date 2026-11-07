import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/data/datasources/pid_schedule.dart';
import 'package:tccelta_mobile/src/data/datasources/pid_scheduler.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';

Duration _ms(int ms) => Duration(milliseconds: ms);

/// Mesmo `twoPids` do `test_acquisition` do firmware: rotação e velocidade.
PidScheduleConfig _twoPids(int tmax, int period, int minInterval) =>
    PidScheduleConfig(
      tmax: _ms(tmax),
      settings: [
        for (final pid in const [Obd2Pid.rpm, Obd2Pid.speed])
          PidSetting(
            pid,
            period: _ms(period),
            minInterval: _ms(minInterval),
          ),
      ],
    );

PidReadDecision _read(PidScheduler s, int now, {int maxPerRead = 1}) {
  final d = s.next(_ms(now), maxPerRead: maxPerRead);
  expect(d, isA<PidReadDecision>(), reason: 'esperava leitura em $now ms');
  return d as PidReadDecision;
}

void main() {
  group('PidScheduleConfig', () {
    test('defaults batem com a Tabela 11 do firmware', () {
      final config = PidScheduleConfig.firmwareDefaults();
      final byPid = {for (final s in config.settings) s.pid: s};

      expect(config.settings, hasLength(7));
      expect(byPid[Obd2Pid.rpm]!.period, _ms(250));
      expect(byPid[Obd2Pid.rpm]!.minInterval, _ms(125));
      expect(byPid[Obd2Pid.engineLoad]!.period, _ms(600));
      expect(byPid[Obd2Pid.fuelTankLevel]!.minInterval, _ms(10000));
      expect(config.utilization, closeTo(0.78, 0.005));
      expect(config.isFeasible, isTrue);
    });

    test('piso de 100 ms em Pᵢ e Pᵢᵐⁱⁿ', () {
      final early = PidSetting.early(Obd2Pid.rpm, _ms(90));
      expect(early.period, _ms(100));
      expect(early.minInterval, _ms(100));

      final config = PidScheduleConfig.firmwareDefaults(tmax: _ms(10));
      expect(config.settings.first.period, _ms(100)); // 5 × 10 ms < piso
    });

    test('um PID a 100 ms torna a tabela inviável', () {
      final defaults = PidScheduleConfig.firmwareDefaults();
      final config = PidScheduleConfig(
        settings: [
          ...defaults.settings,
          PidSetting.early(Obd2Pid.throttle, _ms(100)), // +0,5
        ],
      );
      expect(config.isFeasible, isFalse);
    });

    test('telemetria: foco no período do firmware e o resto do catálogo '
        'lento', () {
      final config = PidScheduleConfig.telemetry(
        focus: const [Obd2Pid.rpm, Obd2Pid.intakeAirTemp],
        catalog: const [Obd2Pid.speed, Obd2Pid.rpm, Obd2Pid.coolantTemp],
      );

      expect(config.settings.map((s) => s.pid), [
        Obd2Pid.rpm,
        Obd2Pid.intakeAirTemp,
        Obd2Pid.speed,
        Obd2Pid.coolantTemp,
      ]);
      expect(config.settings[0], PidSetting.early(Obd2Pid.rpm, _ms(250)));
      // Fora da Tabela 10: 20 × Tmax, com leitura antecipada.
      expect(
        config.settings[1],
        PidSetting.early(Obd2Pid.intakeAirTemp, _ms(1000)),
      );
      expect(
        config.settings[2],
        PidSetting.slow(Obd2Pid.speed, kCatalogPidPeriod),
      );
    });

    test('PID repetido fica só com a primeira linha', () {
      final config = PidScheduleConfig(
        settings: [
          PidSetting.slow(Obd2Pid.rpm, _ms(250)),
          PidSetting.slow(Obd2Pid.rpm, _ms(1000)),
        ],
      );
      expect(config.settings, [PidSetting.slow(Obd2Pid.rpm, _ms(250))]);
    });
  });

  group('PidScheduler (portado do firmware)', () {
    test('lê primeiro o mais atrasado', () {
      final s = PidScheduler()..apply(_twoPids(50, 250, 250), Duration.zero);

      var d = _read(s, 0);
      expect(d.early, isFalse);
      expect(d.pids, [Obd2Pid.rpm]);
      s.onRead(d, Duration.zero);

      d = _read(s, 0);
      expect(d.pids, [Obd2Pid.speed]);
    });

    test('espera quando nada venceu e não há leitura antecipada', () {
      final s = PidScheduler()..apply(_twoPids(50, 250, 250), Duration.zero);
      s
        ..onRead(_read(s, 0), Duration.zero)
        ..onRead(_read(s, 0), Duration.zero);

      final d = s.next(_ms(10));
      expect(d, isA<PidWaitDecision>());
      expect((d as PidWaitDecision).wait, _ms(240));
    });

    test('antecipa só com folga maior que Tmax', () {
      PidScheduler fresh() {
        final s = PidScheduler()..apply(_twoPids(50, 400, 100), Duration.zero);
        return s
          ..onRead(_read(s, 0), Duration.zero) // rpm: próxima em 400
          ..onRead(_read(s, 0), Duration.zero); // speed: próxima em 400
      }

      // Em 100: Pmin cumprido e folga de 300 > Tmax → antecipa.
      final d = _read(fresh(), 100);
      expect(d.early, isTrue);
      expect(d.slack, _ms(300));

      // Em 360: folga de 40 < Tmax → não antecipa, espera o vencimento.
      final wait = fresh().next(_ms(360));
      expect(wait, isA<PidWaitDecision>());
      expect((wait as PidWaitDecision).wait, _ms(40));
    });

    test('mantém a fase das leituras vencidas', () {
      final s = PidScheduler()..apply(_twoPids(50, 250, 250), Duration.zero);
      s
        ..onRead(_read(s, 0), _ms(30)) // lida com atraso de 30 ms
        ..onRead(_read(s, 30), _ms(60));

      // nextDue += P (250), não now + P (280/310): os dois vencem juntos.
      final d = _read(s, 250, maxPerRead: 2);
      expect(d.pids, hasLength(2));
    });

    test('leitura antecipada reagenda a partir do fim da leitura', () {
      final s = PidScheduler()..apply(_twoPids(50, 400, 100), Duration.zero);
      s
        ..onRead(_read(s, 0), Duration.zero)
        ..onRead(_read(s, 0), Duration.zero);

      final d = _read(s, 100);
      s.onRead(d, _ms(150));
      expect(s.nextDue(d.pids.single), _ms(550));
    });

    test('intervalo entre leituras ≤ Pᵢ + Σⱼ≠ᵢ Tⱼ no pior caso', () {
      // Todas as respostas demoram Tmax (canal sequencial no pior caso).
      final config = PidScheduleConfig.firmwareDefaults();
      final s = PidScheduler()..apply(config, Duration.zero);

      var now = Duration.zero;
      final lastStart = <Obd2Pid, Duration>{};
      final worstGap = <Obd2Pid, Duration>{};
      for (var step = 0; step < 2000; step++) {
        switch (s.next(now)) {
          case PidWaitDecision(:final wait):
            now += wait == Duration.zero ? _ms(1) : wait;
          case final PidReadDecision d:
            final pid = d.pids.single;
            final start = now;
            if (lastStart[pid] case final last?) {
              final gap = start - last;
              if (gap > (worstGap[pid] ?? Duration.zero)) worstGap[pid] = gap;
            }
            lastStart[pid] = start;
            now += config.tmax;
            s.onRead(d, now);
        }
      }

      final others = config.tmax * (config.settings.length - 1);
      for (final setting in config.settings) {
        expect(worstGap, contains(setting.pid));
        expect(
          worstGap[setting.pid]! <= setting.period + others,
          isTrue,
          reason:
              '${setting.pid.name}: ${worstGap[setting.pid]} > '
              '${setting.period + others}',
        );
      }
    });

    test('sem PIDs espera 1 s', () {
      final d = PidScheduler().next(Duration.zero);
      expect((d as PidWaitDecision).wait, kPidSchedulerIdleWait);
    });
  });

  group('PidScheduler.apply (diferença do firmware)', () {
    test('PID que continua mantém o vencimento; PID novo vence já', () {
      final s = PidScheduler()
        ..apply(
          PidScheduleConfig(settings: [PidSetting.slow(Obd2Pid.rpm, _ms(250))]),
          Duration.zero,
        );
      s
        ..onRead(_read(s, 0), _ms(50))
        ..apply(
          PidScheduleConfig(
            settings: [
              PidSetting.slow(Obd2Pid.rpm, _ms(250)),
              PidSetting.slow(Obd2Pid.speed, _ms(250)),
            ],
          ),
          _ms(100),
        );

      expect(s.nextDue(Obd2Pid.rpm), _ms(250));
      expect(s.nextDue(Obd2Pid.speed), _ms(100));
      expect(_read(s, 100).pids, [Obd2Pid.speed]);
    });

    test('período novo conta a partir da última leitura', () {
      final s = PidScheduler()
        ..apply(
          PidScheduleConfig(
            settings: [PidSetting.slow(Obd2Pid.rpm, kCatalogPidPeriod)],
          ),
          Duration.zero,
        );
      s
        ..onRead(_read(s, 0), _ms(50))
        // Entrou no Painel: passa de 5 s para 250 ms.
        ..apply(
          PidScheduleConfig.telemetry(focus: const [Obd2Pid.rpm]),
          _ms(100),
        );

      expect(s.nextDue(Obd2Pid.rpm), _ms(300));
    });

    test('PID removido some da tabela e o reagendamento o ignora', () {
      final s = PidScheduler()..apply(_twoPids(50, 250, 250), Duration.zero);
      final d = _read(s, 0);
      s
        ..apply(
          PidScheduleConfig(
            settings: [PidSetting.slow(Obd2Pid.speed, _ms(250))],
          ),
          Duration.zero,
        )
        ..onRead(d, _ms(50)); // d leu rpm, que saiu

      expect(s.length, 1);
      expect(s.nextDue(Obd2Pid.rpm), isNull);
    });
  });
}
