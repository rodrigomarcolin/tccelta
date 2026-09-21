import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';
import 'package:tccelta_mobile/src/domain/ble/security_mode.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_snapshot.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_adapter_info.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_reading.dart';
import 'package:tccelta_mobile/src/domain/repositories/obd2_repository.dart';
import 'package:tccelta_mobile/src/ui/connection/connection_providers.dart';
import 'package:tccelta_mobile/src/ui/connection/view/connecting_screen.dart';
import 'package:tccelta_mobile/src/ui/connection/view_model/connecting_view_model.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';
import 'package:tccelta_mobile/src/ui/telemetry/telemetry_providers.dart';

import '../../support/fake_ble_service.dart';

/// PSK de 64 chars hex (32 bytes) só para satisfazer o gate do
/// `SecureDongleDatasource` nos testes — o valor em si é irrelevante, já que
/// o [FakeBleService] não fala o protocolo real do dongle.
String get _fakePskHex => List.filled(32, 'ab').join();

/// Repository OBD-II fake e instantâneo, para não depender de trocas ELM327
/// reais durante a preparação (Fase 2).
class _FakeObd2Repository implements Obd2Repository {
  @override
  List<Obd2Pid> get pids => Obd2Pid.values;

  @override
  Obd2AdapterInfo? get adapterInfo => null;

  @override
  Future<void> initialize() async {}

  @override
  Future<Set<Obd2Pid>> discoverSupported() async => {Obd2Pid.rpm};

  @override
  Future<Obd2Reading> read(Obd2Pid pid) async =>
      const Obd2Reading(pid: Obd2Pid.rpm, value: 1500);

  @override
  Future<List<Obd2Reading>> readAll() async => const [];

  @override
  Future<List<Obd2Reading>> readMany(List<Obd2Pid> pids) async => const [];

  @override
  Future<DtcSnapshot> readDtc() async =>
      const DtcSnapshot(active: [], milOn: false);
}

void main() {
  group('ConnectingViewModel', () {
    setUp(() {
      // SecureDongleDatasource exige uma PSK salva para liberar a conexão
      // (ver secure_dongle_datasource.dart) — sem isso, todo connect() falha
      // com "PSK não configurada", mascarando o que estes testes exercitam.
      // Modo staticPsk dispensa o handshake ELM327 (que o FakeBleService não
      // simula): a conexão criptografada fica pronta assim que a interna
      // chega em `ready`.
      SharedPreferences.setMockInitialValues({
        'psk_hex': _fakePskHex,
        'security_mode': SecurityMode.staticPsk.storageValue,
      });
    });

    test('connect avança BLE + preparação OBD-II até prep done', () async {
      final container = ProviderContainer(
        overrides: [
          bleServiceProvider.overrideWithValue(FakeBleService()),
          obd2RepositoryProvider.overrideWithValue(_FakeObd2Repository()),
        ],
      );
      addTearDown(container.dispose);

      await container.read(connectingViewModelProvider.notifier).connect('1');
      await Future<void>.delayed(const Duration(milliseconds: 20));

      final state = container.read(connectingViewModelProvider);
      expect(state.phase, BleConnectionPhase.ready);
      expect(state.prep, ConnectingPrep.done);
    });

    test('reconexão reinicia a preparação (não fica com prep done)', () async {
      final container = ProviderContainer(
        overrides: [
          bleServiceProvider.overrideWithValue(FakeBleService()),
          obd2RepositoryProvider.overrideWithValue(_FakeObd2Repository()),
        ],
      );
      addTearDown(container.dispose);

      final notifier = container.read(connectingViewModelProvider.notifier);

      // 1ª conexão até concluir a preparação.
      await notifier.connect('1');
      await Future<void>.delayed(const Duration(milliseconds: 20));
      expect(
        container.read(connectingViewModelProvider).prep,
        ConnectingPrep.done,
      );

      // Reconecta: o estado deve zerar de imediato (prep volta a idle), não
      // ficar preso no `done` da sessão anterior.
      final future = notifier.connect('1');
      expect(
        container.read(connectingViewModelProvider).prep,
        ConnectingPrep.idle,
      );

      // E a preparação reexecuta até done novamente.
      await future;
      await Future<void>.delayed(const Duration(milliseconds: 20));
      final state = container.read(connectingViewModelProvider);
      expect(state.phase, BleConnectionPhase.ready);
      expect(state.prep, ConnectingPrep.done);
    });

    test('falha na conexão vira fase failed (sem preparação)', () async {
      final container = ProviderContainer(
        overrides: [
          bleServiceProvider.overrideWithValue(
            FakeBleService(phases: const [BleConnectionPhase.failed]),
          ),
          obd2RepositoryProvider.overrideWithValue(_FakeObd2Repository()),
        ],
      );
      addTearDown(container.dispose);

      await container.read(connectingViewModelProvider.notifier).connect('1');
      await Future<void>.delayed(const Duration(milliseconds: 20));

      final state = container.read(connectingViewModelProvider);
      expect(state.phase, BleConnectionPhase.failed);
      expect(state.prep, ConnectingPrep.idle);
    });
  });

  group('connectingStepsFor', () {
    test('connecting deixa o 1º passo ativo e os demais pendentes', () {
      final steps = connectingStepsFor(
        const ConnectingState(phase: BleConnectionPhase.connecting),
      );
      expect(steps[0].state, ConnectStepState.active);
      expect(steps[1].state, ConnectStepState.pending);
      expect(steps[2].state, ConnectStepState.pending);
      expect(steps[3].state, ConnectStepState.pending);
    });

    test('ready + preparação em curso: passo 3 ativo, 4 pendente', () {
      final steps = connectingStepsFor(
        const ConnectingState(
          phase: BleConnectionPhase.ready,
          prep: ConnectingPrep.preparing,
        ),
      );
      expect(steps[0].state, ConnectStepState.done);
      expect(steps[1].state, ConnectStepState.done);
      expect(steps[2].state, ConnectStepState.active);
      expect(steps[3].state, ConnectStepState.pending);
    });

    test('ready + leitura de capacidades: passo 3 done, 4 ativo', () {
      final steps = connectingStepsFor(
        const ConnectingState(
          phase: BleConnectionPhase.ready,
          prep: ConnectingPrep.reading,
        ),
      );
      expect(steps[2].state, ConnectStepState.done);
      expect(steps[3].state, ConnectStepState.active);
    });

    test('preparação concluída: todos os 4 passos done', () {
      final steps = connectingStepsFor(
        const ConnectingState(
          phase: BleConnectionPhase.ready,
          prep: ConnectingPrep.done,
        ),
      );
      expect(steps.every((s) => s.state == ConnectStepState.done), isTrue);
    });
  });
}
