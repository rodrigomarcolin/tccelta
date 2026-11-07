import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/core/errors/obd_failure.dart';
import 'package:tccelta_mobile/src/domain/location/gps_fix.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_snapshot.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_adapter_info.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_reading.dart';
import 'package:tccelta_mobile/src/domain/repositories/obd2_repository.dart';
import 'package:tccelta_mobile/src/domain/telemetry/indicator_display.dart';
import 'package:tccelta_mobile/src/ui/telemetry/telemetry_providers.dart';
import 'package:tccelta_mobile/src/ui/telemetry/view_model/panel_view_model.dart';
import 'package:tccelta_mobile/src/ui/telemetry/view_model/telemetry_view_model.dart';

import '../../support/fake_panel_repository.dart';

/// Repository de telemetria fake: a leitura agendada é um controller que o
/// teste alimenta, e cada [setFocus] fica registrado.
class _FakeObd2Repository implements Obd2Repository {
  final StreamController<List<Obd2Reading>> readingsCtrl =
      StreamController<List<Obd2Reading>>();

  final List<Set<Obd2Pid>> focusCalls = [];

  int watchCalls = 0;

  @override
  Future<void> reportGps(GpsFix fix) async {}

  @override
  List<Obd2Pid> get pids => Obd2Pid.values;

  @override
  Obd2AdapterInfo? get adapterInfo => null;

  @override
  Future<void> initialize() async {}

  @override
  Future<Set<Obd2Pid>> discoverSupported() async => {
    Obd2Pid.rpm,
    Obd2Pid.speed,
  };

  @override
  Future<Obd2Reading> read(Obd2Pid pid) async =>
      const Obd2Reading(pid: Obd2Pid.rpm, value: 1500);

  @override
  Future<List<Obd2Reading>> readAll() async => const [];

  @override
  Future<List<Obd2Reading>> readMany(List<Obd2Pid> pids) async => const [];

  @override
  Stream<List<Obd2Reading>> watchReadings() {
    watchCalls++;
    return readingsCtrl.stream;
  }

  @override
  void setFocus(Iterable<Obd2Pid> focus) => focusCalls.add(focus.toSet());

  @override
  Future<DtcSnapshot> readDtc() async =>
      const DtcSnapshot(active: [], milOn: false);
}

Future<void> _settle() =>
    Future<void>.delayed(const Duration(milliseconds: 10));

void main() {
  late _FakeObd2Repository repo;
  late ProviderContainer container;

  setUp(() {
    repo = _FakeObd2Repository();
    container = ProviderContainer(
      overrides: [
        obd2RepositoryProvider.overrideWithValue(repo),
        panelRepositoryProvider.overrideWithValue(FakePanelRepository()),
      ],
    );
    addTearDown(container.dispose);
  });

  test('a leitura agendada popula as leituras e o histórico', () async {
    container.listen(telemetryViewModelProvider, (_, _) {});

    expect(container.read(telemetryViewModelProvider).isPolling, isTrue);
    expect(container.read(telemetryViewModelProvider).readings, isEmpty);

    await _settle();
    expect(repo.watchCalls, 1);
    expect(container.read(telemetryViewModelProvider).supportedPids, {
      Obd2Pid.rpm,
      Obd2Pid.speed,
    });

    repo.readingsCtrl
      ..add(const [Obd2Reading(pid: Obd2Pid.rpm, value: 1500)])
      ..add(const [Obd2Reading(pid: Obd2Pid.speed, value: 60)])
      ..add(const [Obd2Reading(pid: Obd2Pid.rpm, value: 1600)]);
    await _settle();

    final state = container.read(telemetryViewModelProvider);
    expect(state.readings.map((r) => (r.pid, r.value)).toSet(), {
      (Obd2Pid.rpm, 1600),
      (Obd2Pid.speed, 60),
    });
    expect(state.history[Obd2Pid.rpm], [1500, 1600]);
    expect(state.failure, isNull);
  });

  test('erro da stream vira falha e a próxima leitura a limpa', () async {
    container.listen(telemetryViewModelProvider, (_, _) {});
    await _settle();

    repo.readingsCtrl.addError(const ObdCommandFailure('Sem conexão'));
    await _settle();
    expect(
      container.read(telemetryViewModelProvider).failure,
      isA<ObdCommandFailure>(),
    );

    repo.readingsCtrl.add(const [Obd2Reading(pid: Obd2Pid.rpm, value: 900)]);
    await _settle();
    expect(container.read(telemetryViewModelProvider).failure, isNull);
  });

  test('o foco segue os indicadores do painel ativo', () async {
    await container.read(panelViewModelProvider.future);
    container.listen(telemetryViewModelProvider, (_, _) {});
    await _settle();

    // Painel vazio: foco vazio (só o catálogo é lido).
    expect(repo.focusCalls.last, isEmpty);

    container
        .read(panelViewModelProvider.notifier)
        .addIndicator(Obd2Pid.rpm, IndicatorDisplay.defaultFor(Obd2Pid.rpm));
    await _settle();
    expect(repo.focusCalls.last, {Obd2Pid.rpm});

    container
        .read(panelViewModelProvider.notifier)
        .addIndicator(
          Obd2Pid.speed,
          IndicatorDisplay.defaultFor(Obd2Pid.speed),
        );
    await _settle();
    expect(repo.focusCalls.last, {Obd2Pid.rpm, Obd2Pid.speed});
  });

  test('descartar o provider cancela a leitura agendada', () async {
    final sub = container.listen(telemetryViewModelProvider, (_, _) {});
    await _settle();
    expect(repo.readingsCtrl.hasListener, isTrue);

    sub.close();
    await _settle();
    expect(repo.readingsCtrl.hasListener, isFalse);
  });
}
