import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_adapter_info.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_reading.dart';
import 'package:tccelta_mobile/src/domain/repositories/obd2_repository.dart';
import 'package:tccelta_mobile/src/ui/connection/view_model/connected_view_model.dart';
import 'package:tccelta_mobile/src/ui/telemetry/telemetry_providers.dart';

/// Repository fake que devolve uma identidade e um conjunto suportado fixos.
class _FakeObd2Repository implements Obd2Repository {
  @override
  List<Obd2Pid> get pids => Obd2Pid.values;

  @override
  Obd2AdapterInfo? get adapterInfo => const Obd2AdapterInfo(
    version: 'ELM327 v1.5',
    protocol: 'ISO 15765-4 (CAN 11/500)',
  );

  @override
  Future<void> initialize() async {}

  @override
  Future<Set<Obd2Pid>> discoverSupported() async => {
    Obd2Pid.rpm,
    Obd2Pid.speed,
    Obd2Pid.coolantTemp,
  };

  @override
  Future<Obd2Reading> read(Obd2Pid pid) async =>
      const Obd2Reading(pid: Obd2Pid.rpm, value: 1500);

  @override
  Future<List<Obd2Reading>> readAll() async => const [];

  @override
  Future<List<Obd2Reading>> readMany(List<Obd2Pid> pids) async => const [];
}

void main() {
  test('a sondagem expõe protocolo e nº de sensores', () async {
    final container = ProviderContainer(
      overrides: [
        obd2RepositoryProvider.overrideWithValue(_FakeObd2Repository()),
      ],
    );
    addTearDown(container.dispose);

    container.listen(connectedViewModelProvider, (_, _) {});

    // Estado inicial: sondando.
    expect(container.read(connectedViewModelProvider).probing, isTrue);

    await Future<void>.delayed(const Duration(milliseconds: 10));

    final state = container.read(connectedViewModelProvider);
    expect(state.probing, isFalse);
    expect(state.info?.protocol, 'ISO 15765-4 (CAN 11/500)');
    expect(state.supported.length, 3);
    expect(state.failure, isNull);
  });
}
