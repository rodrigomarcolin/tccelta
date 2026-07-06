import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_reading.dart';
import 'package:tccelta_mobile/src/domain/repositories/obd2_repository.dart';
import 'package:tccelta_mobile/src/ui/telemetry/telemetry_providers.dart';
import 'package:tccelta_mobile/src/ui/telemetry/view_model/telemetry_view_model.dart';

/// Repository de telemetria fake que devolve leituras fixas.
class _FakeObd2Repository implements Obd2Repository {
  @override
  List<Obd2Pid> get pids => Obd2Pid.values;

  @override
  Future<void> initialize() async {}

  @override
  Future<Obd2Reading> read(Obd2Pid pid) async =>
      const Obd2Reading(pid: Obd2Pid.rpm, value: 1500);

  @override
  Future<List<Obd2Reading>> readAll() async => const [
        Obd2Reading(pid: Obd2Pid.rpm, value: 1500),
        Obd2Reading(pid: Obd2Pid.speed, value: 60),
      ];
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

    // Deixa o primeiro ciclo (readAll) resolver.
    await Future<void>.delayed(const Duration(milliseconds: 10));

    final state = container.read(telemetryViewModelProvider);
    expect(state.readings.length, 2);
    expect(state.readings.first.value, 1500);
    expect(state.failure, isNull);
  });
}
