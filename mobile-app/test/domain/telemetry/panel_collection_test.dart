import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';
import 'package:tccelta_mobile/src/domain/telemetry/indicator_display.dart';
import 'package:tccelta_mobile/src/domain/telemetry/panel.dart';
import 'package:tccelta_mobile/src/domain/telemetry/panel_collection.dart';

void main() {
  group('PanelsState — atalhos para o painel ativo', () {
    test('active resolve o painel cujo id é activeId', () {
      const state = PanelsState(
        panels: [
          Panel(id: 'p1', name: 'Painel 1'),
          Panel(id: 'p2', name: 'Painel 2'),
        ],
        activeId: 'p2',
      );

      expect(state.active.name, 'Painel 2');
    });

    test('indicators/contains/displayFor delegam para o painel ativo', () {
      final display = IndicatorDisplay.defaultFor(Obd2Pid.rpm);
      final state = PanelsState(
        panels: [
          Panel(
            id: 'p1',
            name: 'Painel 1',
            indicatorIds: const ['rpm'],
            displays: {'rpm': display},
          ),
        ],
        activeId: 'p1',
      );

      expect(state.indicators, [Obd2Pid.rpm]);
      expect(state.contains(Obd2Pid.rpm), isTrue);
      expect(state.displayFor(Obd2Pid.rpm), display);
    });
  });

  group('PanelsState — copyWith', () {
    test('sobrescreve só os campos informados', () {
      const base = PanelsState(
        panels: [Panel(id: 'p1', name: 'Painel 1')],
        activeId: 'p1',
      );

      final renamed = base.copyWith(activeId: 'p1');

      expect(renamed.panels, base.panels);
      expect(renamed.activeId, 'p1');
    });
  });

  group('PanelsState — serialização', () {
    test('toJson serializa panels e activeId', () {
      const state = PanelsState(
        panels: [Panel(id: 'p1', name: 'Painel 1')],
        activeId: 'p1',
      );

      final json = state.toJson();

      expect(json['activeId'], 'p1');
      expect(json['panels'], [
        {
          'id': 'p1',
          'name': 'Painel 1',
          'indicatorIds': <String>[],
          'displays': <String, dynamic>{},
        },
      ]);
    });

    test('fromJson reconstrói um PanelsState equivalente ao original', () {
      final display = IndicatorDisplay.defaultFor(Obd2Pid.rpm);
      final state = PanelsState(
        panels: [
          Panel(
            id: 'p1',
            name: 'Painel 1',
            indicatorIds: const ['rpm'],
            displays: {'rpm': display},
          ),
          const Panel(id: 'p2', name: 'Painel 2'),
        ],
        activeId: 'p2',
      );

      final rebuilt = PanelsState.fromJson(state.toJson());

      expect(rebuilt.activeId, 'p2');
      expect(rebuilt.panels.map((p) => p.id), ['p1', 'p2']);
      expect(rebuilt.panels.first.displayFor(Obd2Pid.rpm), display);
    });
  });
}
