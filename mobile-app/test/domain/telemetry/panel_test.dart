import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';
import 'package:tccelta_mobile/src/domain/telemetry/indicator_display.dart';
import 'package:tccelta_mobile/src/domain/telemetry/panel.dart';

void main() {
  group('Panel — indicadores', () {
    test('painel novo não tem indicadores', () {
      const panel = Panel(id: 'p1', name: 'Painel 1');

      expect(panel.indicators, isEmpty);
      expect(panel.contains(Obd2Pid.rpm), isFalse);
    });

    test('indicators mapeia indicatorIds de volta para Obd2Pid, na ordem', () {
      const panel = Panel(
        id: 'p1',
        name: 'Painel 1',
        indicatorIds: ['speed', 'rpm'],
      );

      expect(panel.indicators, [Obd2Pid.speed, Obd2Pid.rpm]);
    });

    test('displayFor cai no default do PID sem customização salva', () {
      const panel = Panel(id: 'p1', name: 'Painel 1');

      expect(
        panel.displayFor(Obd2Pid.rpm),
        IndicatorDisplay.defaultFor(Obd2Pid.rpm),
      );
    });

    test('displayFor usa a customização salva quando existe', () {
      final display = IndicatorDisplay.defaultFor(
        Obd2Pid.rpm,
      ).copyWith(format: IndicatorFormat.gauge);
      final panel = Panel(
        id: 'p1',
        name: 'Painel 1',
        indicatorIds: const ['rpm'],
        displays: {'rpm': display},
      );

      expect(panel.displayFor(Obd2Pid.rpm), display);
    });
  });

  group('Panel — copyWith', () {
    test('sobrescreve só os campos informados e preserva o id', () {
      const base = Panel(id: 'p1', name: 'Painel 1');

      final renamed = base.copyWith(name: 'Pista');

      expect(renamed.id, 'p1');
      expect(renamed.name, 'Pista');
      expect(renamed.indicatorIds, base.indicatorIds);
      expect(renamed.displays, base.displays);
    });
  });

  group('Panel — serialização', () {
    test('toJson serializa id, nome, indicatorIds e displays', () {
      final display = IndicatorDisplay.defaultFor(Obd2Pid.rpm);
      final panel = Panel(
        id: 'p1',
        name: 'Uso diário',
        indicatorIds: const ['rpm'],
        displays: {'rpm': display},
      );

      final json = panel.toJson();

      expect(json['id'], 'p1');
      expect(json['name'], 'Uso diário');
      expect(json['indicatorIds'], ['rpm']);
      expect(json['displays'], {'rpm': display.toJson()});
    });
  });
}
