import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:tccelta_mobile/src/core/persistence/shared_preferences_provider.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_bond_state.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_device.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_link_error.dart';
import 'package:tccelta_mobile/src/domain/ble/dongle_pairing_hint.dart';
import 'package:tccelta_mobile/src/domain/ble/last_dongle.dart';
import 'package:tccelta_mobile/src/domain/location/gps_fix.dart';
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

/// Repository OBD-II fake e instantâneo, para não depender de trocas ELM327
/// reais durante a preparação.
class _FakeObd2Repository implements Obd2Repository {
  @override
  Future<void> reportGps(GpsFix fix) async {}

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

BleDevice _frame({required bool windowOpen}) => BleDevice(
  id: '1',
  name: 'TCCeltaDongle',
  rssi: -50,
  manufacturerData: manufacturerFor(windowOpen: windowOpen),
);

FakeConnectPlan _failingPlan(BleLinkError error) => FakeConnectPlan(const [
  BleConnectionPhase.connecting,
  BleConnectionPhase.securing,
  BleConnectionPhase.failed,
], error: error);

void main() {
  late SharedPreferences prefs;

  setUp(() async {
    SharedPreferences.setMockInitialValues({});
    prefs = await SharedPreferences.getInstance();
  });

  ProviderContainer makeContainer(FakeBleService ble) {
    final container = ProviderContainer(
      overrides: [
        bleServiceProvider.overrideWithValue(ble),
        obd2RepositoryProvider.overrideWithValue(_FakeObd2Repository()),
        sharedPreferencesProvider.overrideWithValue(prefs),
      ],
    );
    addTearDown(container.dispose);
    return container;
  }

  Future<void> until(
    ProviderContainer container,
    bool Function(ConnectingState) test,
  ) async {
    for (var i = 0; i < 300; i++) {
      if (test(container.read(connectingViewModelProvider))) return;
      await Future<void>.delayed(const Duration(milliseconds: 10));
    }
    final s = container.read(connectingViewModelProvider);
    fail('estado esperado não alcançado: ${s.security} / ${s.phase}');
  }

  const openHint = DonglePairingHint(windowOpen: true, hasBond: false);

  group('ConnectingViewModel', () {
    test('com bond restaura a sessão e conclui a preparação OBD-II', () async {
      final container = makeContainer(
        FakeBleService(bondStates: {'1': BleBondState.bonded}),
      );
      await container.read(connectingViewModelProvider.notifier).connect('1');
      await until(container, (s) => s.prep == ConnectingPrep.done);
      final state = container.read(connectingViewModelProvider);
      expect(state.phase, BleConnectionPhase.ready);
      expect(state.security, LinkSecurityStep.secured);
    });

    test('janela aberta sem bond: pareia agora e lembra o dongle', () async {
      final ble = FakeBleService();
      final container = makeContainer(ble);
      container
          .read(selectedDongleProvider.notifier)
          .select(const BleDevice(id: '1', name: 'OBD2Dongle', rssi: -50));
      final phases = <BleConnectionPhase>[];
      container.listen(
        connectingViewModelProvider.select((s) => s.phase),
        (_, next) => phases.add(next),
      );
      await container
          .read(connectingViewModelProvider.notifier)
          .connect('1', hint: openHint);
      await until(container, (s) => s.prep == ConnectingPrep.done);
      expect(phases, contains(BleConnectionPhase.securing));
      expect(ble.connectCount, 1);
      final saved = await container.read(lastDongleRepositoryProvider).load();
      expect(saved?.id, '1');
      expect(saved?.name, 'OBD2Dongle');
    });

    test('janela fechada: espera o botão e pareia quando ela abre', () async {
      final container = makeContainer(
        FakeBleService(
          hintFrames: [
            [_frame(windowOpen: false)],
            [_frame(windowOpen: true)],
          ],
        ),
      );
      await container.read(connectingViewModelProvider.notifier).connect('1');
      await until(
        container,
        (s) => s.security == LinkSecurityStep.waitingButton && s.dongleSeen,
      );
      await until(container, (s) => s.security == LinkSecurityStep.secured);
    });

    test('Android: bond perdido remove o bond e pareia de novo', () async {
      final ble = FakeBleService(
        bondStates: {'1': BleBondState.bonded},
        plans: [_failingPlan(BleLinkError.bondLost)],
        hintFrames: [
          [_frame(windowOpen: true)],
        ],
      );
      final container = makeContainer(ble);
      await container.read(connectingViewModelProvider.notifier).connect('1');
      await until(container, (s) => s.security == LinkSecurityStep.secured);
      expect(ble.removeBondCalls, ['1']);
      expect(ble.connectCount, 2);
    });

    test('iOS: bond perdido orienta esquecer e apaga o registro', () async {
      final ble = FakeBleService(
        canManageBonds: false,
        plans: [_failingPlan(BleLinkError.bondLostIos)],
      );
      final container = makeContainer(ble);
      await container
          .read(lastDongleRepositoryProvider)
          .save(const LastDongle(id: '1', name: 'D'));
      await container.read(connectingViewModelProvider.notifier).connect('1');
      await until(container, (s) => s.security == LinkSecurityStep.forgetOnIos);
      await Future<void>.delayed(const Duration(milliseconds: 20));
      expect(await container.read(lastDongleRepositoryProvider).load(), isNull);
      expect(ble.removeBondCalls, isEmpty);
    });

    test('recusa com a janela ainda aberta vira rejected', () async {
      final container = makeContainer(
        FakeBleService(
          plans: [_failingPlan(BleLinkError.pairingRejected)],
          hintFrames: [
            [_frame(windowOpen: true)],
          ],
        ),
      );
      await container
          .read(connectingViewModelProvider.notifier)
          .connect('1', hint: openHint);
      await until(container, (s) => s.security == LinkSecurityStep.rejected);
    });

    test('recusa com a janela fechada vira windowExpired', () async {
      final container = makeContainer(
        FakeBleService(
          plans: [_failingPlan(BleLinkError.notAuthorized)],
          hintFrames: [
            [_frame(windowOpen: false)],
          ],
        ),
      );
      await container
          .read(connectingViewModelProvider.notifier)
          .connect('1', hint: openHint);
      await until(
        container,
        (s) => s.security == LinkSecurityStep.windowExpired,
      );
    });

    test('iOS: "já pareei" tenta restaurar com o bond do SO', () async {
      final container = makeContainer(FakeBleService(canManageBonds: false));
      final notifier = container.read(connectingViewModelProvider.notifier);
      await notifier.connect('1');
      expect(
        container.read(connectingViewModelProvider).security,
        LinkSecurityStep.waitingButton,
      );
      await notifier.assumePaired();
      await until(container, (s) => s.security == LinkSecurityStep.secured);
    });

    test('falha genérica com bond existente vira failed', () async {
      final container = makeContainer(
        FakeBleService(
          bondStates: {'1': BleBondState.bonded},
          phases: const [BleConnectionPhase.failed],
        ),
      );
      await container.read(connectingViewModelProvider.notifier).connect('1');
      await until(container, (s) => s.security == LinkSecurityStep.failed);
      final state = container.read(connectingViewModelProvider);
      expect(state.phase, BleConnectionPhase.failed);
      expect(state.prep, ConnectingPrep.idle);
    });

    test('reconectar reinicia a preparação (não fica com prep done)', () async {
      final container = makeContainer(
        FakeBleService(bondStates: {'1': BleBondState.bonded}),
      );
      final notifier = container.read(connectingViewModelProvider.notifier);
      await notifier.connect('1');
      await until(container, (s) => s.prep == ConnectingPrep.done);
      final future = notifier.connect('1');
      expect(
        container.read(connectingViewModelProvider).prep,
        ConnectingPrep.idle,
      );
      await future;
      await until(container, (s) => s.prep == ConnectingPrep.done);
    });
  });

  group('connectingStepsFor', () {
    test('connecting deixa o 1º passo ativo e os demais pendentes', () {
      final steps = connectingStepsFor(
        const ConnectingState(phase: BleConnectionPhase.connecting),
      );
      expect(steps[0].state, ConnectStepState.active);
      expect(
        steps.skip(1).every((s) => s.state == ConnectStepState.pending),
        isTrue,
      );
    });

    test('securing deixa o passo de pareamento ativo', () {
      final steps = connectingStepsFor(
        const ConnectingState(phase: BleConnectionPhase.securing),
      );
      expect(steps[0].state, ConnectStepState.done);
      expect(steps[1].state, ConnectStepState.active);
      expect(steps[2].state, ConnectStepState.pending);
    });

    test('ready + preparação em curso: passo 4 ativo, 5 pendente', () {
      final steps = connectingStepsFor(
        const ConnectingState(
          phase: BleConnectionPhase.ready,
          prep: ConnectingPrep.preparing,
        ),
      );
      expect(steps[2].state, ConnectStepState.done);
      expect(steps[3].state, ConnectStepState.active);
      expect(steps[4].state, ConnectStepState.pending);
    });

    test('ready + leitura de capacidades: passo 4 done, 5 ativo', () {
      final steps = connectingStepsFor(
        const ConnectingState(
          phase: BleConnectionPhase.ready,
          prep: ConnectingPrep.reading,
        ),
      );
      expect(steps[3].state, ConnectStepState.done);
      expect(steps[4].state, ConnectStepState.active);
    });

    test('preparação concluída: todos os 5 passos done', () {
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
