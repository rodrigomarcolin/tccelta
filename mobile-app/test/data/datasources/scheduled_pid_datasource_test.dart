import 'dart:async';

import 'package:fake_async/fake_async.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/core/errors/obd_failure.dart';
import 'package:tccelta_mobile/src/data/datasources/pid_schedule.dart';
import 'package:tccelta_mobile/src/data/datasources/scheduled_pid_datasource.dart';
import 'package:tccelta_mobile/src/domain/obd2/elm_response.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';

import '../../support/fake_monotonic_clock.dart';

Duration _ms(int ms) => Duration(milliseconds: ms);

/// Leitor fake: cada leitura leva [latency]; registra o instante de início.
class _FakeReader implements PidReader {
  _FakeReader(this._clock);

  final FakeMonotonicClock _clock;
  static const Duration latency = Duration(milliseconds: 50);

  /// PIDs que respondem vazio (`NO DATA`).
  final Set<Obd2Pid> silent = {};

  /// Se definido, toda leitura lança isto (sem conexão).
  Exception? unavailable;

  final List<(Obd2Pid, Duration)> starts = [];

  int count(Obd2Pid pid) => starts.where((s) => s.$1 == pid).length;

  @override
  Future<List<ElmResponse>> read(Obd2Pid pid) async {
    starts.add((pid, _clock.now()));
    final error = unavailable;
    if (error != null) throw error;
    await Future<void>.delayed(latency);
    if (silent.contains(pid)) return const [];
    return [
      ElmResponse(
        service: 0x41,
        pid: pid.pid,
        payload: const [0x10, 0x00],
        rawLine: '',
      ),
    ];
  }
}

PidScheduleConfig _config(List<PidSetting> settings) =>
    PidScheduleConfig(settings: settings);

void main() {
  late FakeMonotonicClock clock;
  late _FakeReader reader;

  ScheduledPidDatasource build(FakeAsync async, PidScheduleConfig config) {
    clock = FakeMonotonicClock(source: () => async.elapsed);
    reader = _FakeReader(clock);
    return ScheduledPidDatasource(reader: reader, config: config, clock: clock);
  }

  test('lê cada PID no seu período', () {
    fakeAsync((async) {
      final source = build(
        async,
        _config([
          PidSetting.slow(Obd2Pid.rpm, _ms(250)),
          PidSetting.slow(Obd2Pid.coolantTemp, _ms(5000)),
        ]),
      );
      final samples = <PidSample>[];
      final sub = source.samples.listen(samples.add);

      async.elapse(_ms(900));

      // rpm em 0, 250, 500, 750; temperatura só em 50 (depois do rpm).
      expect(
        [
          for (final (pid, at) in reader.starts)
            if (pid == Obd2Pid.rpm) at,
        ],
        [_ms(0), _ms(250), _ms(500), _ms(750)],
      );
      expect(reader.count(Obd2Pid.coolantTemp), 1);
      expect(reader.starts[1], (Obd2Pid.coolantTemp, _ms(50)));
      expect(samples.map((s) => s.pid), contains(Obd2Pid.coolantTemp));
      expect(samples.first.readAt, _ms(50));

      unawaited(sub.cancel());
      async.flushMicrotasks();
    });
  });

  test('PID sem resposta é reagendado, mas não vira amostra', () {
    fakeAsync((async) {
      final source = build(
        async,
        _config([PidSetting.slow(Obd2Pid.rpm, _ms(250))]),
      );
      reader.silent.add(Obd2Pid.rpm);
      final samples = <PidSample>[];
      final sub = source.samples.listen(samples.add);

      async.elapse(_ms(600));

      expect(reader.count(Obd2Pid.rpm), 3); // 0, 250, 500
      expect(samples, isEmpty);

      unawaited(sub.cancel());
      async.flushMicrotasks();
    });
  });

  test('sem conexão: repassa o erro e tenta de novo depois de 1 s', () {
    fakeAsync((async) {
      final source = build(
        async,
        _config([PidSetting.slow(Obd2Pid.rpm, _ms(250))]),
      );
      reader.unavailable = const ObdCommandFailure('Sem conexão BLE pronta');
      final errors = <Object>[];
      final samples = <PidSample>[];
      final sub = source.samples.listen(samples.add, onError: errors.add);

      async.elapse(_ms(1500));
      expect(reader.starts.map((s) => s.$2), [_ms(0), _ms(1000)]);
      expect(errors, hasLength(2));
      expect(errors.first, isA<ObdCommandFailure>());

      // Conexão volta: o PID, que não foi reagendado, é lido logo.
      reader.unavailable = null;
      async.elapse(_ms(600));
      expect(reader.starts[2], (Obd2Pid.rpm, _ms(2000)));
      expect(samples, isNotEmpty);

      unawaited(sub.cancel());
      async.flushMicrotasks();
    });
  });

  test('reconfigure acorda o laço e o PID novo é lido na hora', () {
    fakeAsync((async) {
      final source = build(
        async,
        _config([PidSetting.slow(Obd2Pid.rpm, _ms(1000))]),
      );
      final sub = source.samples.listen((_) {});

      async.elapse(_ms(100)); // rpm lido em 0; o laço dorme até 1000
      source.reconfigure(
        _config([
          PidSetting.slow(Obd2Pid.rpm, _ms(1000)),
          PidSetting.slow(Obd2Pid.speed, _ms(1000)),
        ]),
      );
      async.flushMicrotasks();

      expect(reader.starts.last, (Obd2Pid.speed, _ms(100)));

      unawaited(sub.cancel());
      async.flushMicrotasks();
    });
  });

  test('cancelar a escuta para o laço', () {
    fakeAsync((async) {
      final source = build(
        async,
        _config([PidSetting.slow(Obd2Pid.rpm, _ms(250))]),
      );
      final sub = source.samples.listen((_) {});
      var done = false;
      unawaited(source.done.then((_) => done = true));

      async.elapse(_ms(300));
      final reads = reader.starts.length;
      unawaited(sub.cancel());
      async.elapse(_ms(2000));

      expect(done, isTrue);
      expect(reader.starts.length, reads);
      expect(async.pendingTimers, isEmpty);
    });
  });

  test('nada é lido antes da escuta', () {
    fakeAsync((async) {
      build(async, _config([PidSetting.slow(Obd2Pid.rpm, _ms(250))]));
      async.elapse(_ms(1000));
      expect(reader.starts, isEmpty);
    });
  });
}
