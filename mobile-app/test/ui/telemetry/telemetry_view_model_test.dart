import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_adapter_info.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_reading.dart';
import 'package:tccelta_mobile/src/domain/repositories/obd2_repository.dart';
import 'package:tccelta_mobile/src/domain/telemetry/indicator_display.dart';
import 'package:tccelta_mobile/src/ui/telemetry/telemetry_providers.dart';
import 'package:tccelta_mobile/src/ui/telemetry/view_model/panel_view_model.dart';
import 'package:tccelta_mobile/src/ui/telemetry/view_model/telemetry_view_model.dart';

/// Repository de telemetria fake que devolve leituras fixas e registra as
/// chamadas de [readMany] (para asserir quais PIDs o ciclo rápido pediu).
class _FakeObd2Repository implements Obd2Repository {
  final List<List<Obd2Pid>> readManyCalls = [];

  @override
  List<Obd2Pid> get pids => Obd2Pid.values;

  @override
  Obd2AdapterInfo? get adapterInfo => null;

  @override
  Future<void> initialize() async {}

  @override
  Future<Set<Obd2Pid>> discoverSupported() async =>
      {Obd2Pid.rpm, Obd2Pid.speed};

  @override
  Future<Obd2Reading> read(Obd2Pid pid) async =>
      const Obd2Reading(pid: Obd2Pid.rpm, value: 1500);

  @override
  Future<List<Obd2Reading>> readAll() async => const [
        Obd2Reading(pid: Obd2Pid.rpm, value: 1500),
        Obd2Reading(pid: Obd2Pid.speed, value: 60),
      ];

  @override
  Future<List<Obd2Reading>> readMany(List<Obd2Pid> pids) async {
    readManyCalls.add(pids);
    final byPid = {for (final r in await readAll()) r.pid: r};
    return [for (final p in pids) ?byPid[p]];
  }
}

void main() {
  test('o polling popula as leituras', () async {
    final container = ProviderContainer(
      overrides: [
        obd2RepositoryProvider.overrideWithValue(_FakeObd2Repository()),
      ],
    );
    addTearDown(container.dispose);

    // Mantém o provider (autoDispose) vivo e dispara o build().
    container.listen(telemetryViewModelProvider, (_, _) {});

    // Estado inicial: ciclo ativo, ainda sem leituras.
    expect(container.read(telemetryViewModelProvider).isPolling, isTrue);
    expect(container.read(telemetryViewModelProvider).readings, isEmpty);

    // Deixa o primeiro ciclo completo (readAll) resolver.
    await Future<void>.delayed(const Duration(milliseconds: 10));

    final state = container.read(telemetryViewModelProvider);
    expect(state.readings.length, 2);
    expect(state.readings.first.value, 1500);
    expect(state.failure, isNull);
  });

  test('painel vazio: o ciclo rápido não chama readMany', () async {
    final repo = _FakeObd2Repository();
    final container = ProviderContainer(
      overrides: [obd2RepositoryProvider.overrideWithValue(repo)],
    );
    addTearDown(container.dispose);

    container.listen(telemetryViewModelProvider, (_, _) {});
    await Future<void>.delayed(const Duration(milliseconds: 10));

    expect(repo.readManyCalls, isEmpty);
  });

  test('ciclo rápido lê só os indicadores do Painel', () async {
    final repo = _FakeObd2Repository();
    final container = ProviderContainer(
      overrides: [obd2RepositoryProvider.overrideWithValue(repo)],
    );
    addTearDown(container.dispose);

    container
        .read(panelViewModelProvider.notifier)
        .addIndicator(Obd2Pid.rpm, IndicatorDisplay.defaultFor(Obd2Pid.rpm));

    container.listen(telemetryViewModelProvider, (_, _) {});
    await Future<void>.delayed(const Duration(milliseconds: 10));

    expect(repo.readManyCalls, isNotEmpty);
    expect(repo.readManyCalls.first, [Obd2Pid.rpm]);

    final state = container.read(telemetryViewModelProvider);
    // O ciclo completo já rodou também: as leituras seguem mescladas, não
    // reduzidas ao subconjunto do Painel.
    expect(state.readings.map((r) => r.pid).toSet(), {
      Obd2Pid.rpm,
      Obd2Pid.speed,
    });
  });
}
