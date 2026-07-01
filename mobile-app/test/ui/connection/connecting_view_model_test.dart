import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';
import 'package:tccelta_mobile/src/ui/connection/connection_providers.dart';
import 'package:tccelta_mobile/src/ui/connection/view/connecting_screen.dart';
import 'package:tccelta_mobile/src/ui/connection/view_model/connecting_view_model.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';

import '../../support/fake_ble_service.dart';

void main() {
  group('ConnectingViewModel', () {
    test('connect avança as fases até ready', () async {
      final container = ProviderContainer(
        overrides: [bleServiceProvider.overrideWithValue(FakeBleService())],
      );
      addTearDown(container.dispose);

      await container.read(connectingViewModelProvider.notifier).connect('1');
      await Future<void>.delayed(const Duration(milliseconds: 20));

      expect(
        container.read(connectingViewModelProvider),
        BleConnectionPhase.ready,
      );
    });

    test('falha na conexão vira fase failed', () async {
      final container = ProviderContainer(
        overrides: [
          bleServiceProvider.overrideWithValue(
            FakeBleService(phases: const [BleConnectionPhase.failed]),
          ),
        ],
      );
      addTearDown(container.dispose);

      await container.read(connectingViewModelProvider.notifier).connect('1');
      await Future<void>.delayed(const Duration(milliseconds: 20));

      expect(
        container.read(connectingViewModelProvider),
        BleConnectionPhase.failed,
      );
    });
  });

  group('connectingStepsFor', () {
    test('connecting deixa o 1º passo ativo e os demais pendentes', () {
      final steps = connectingStepsFor(BleConnectionPhase.connecting);
      expect(steps[0].state, ConnectStepState.active);
      expect(steps[1].state, ConnectStepState.pending);
    });

    test('ready conclui os 2 passos de BLE; os de Fase 2 seguem pendentes', () {
      final steps = connectingStepsFor(BleConnectionPhase.ready);
      expect(steps[0].state, ConnectStepState.done);
      expect(steps[1].state, ConnectStepState.done);
      expect(steps[2].state, ConnectStepState.pending);
      expect(steps[3].state, ConnectStepState.pending);
    });
  });
}
