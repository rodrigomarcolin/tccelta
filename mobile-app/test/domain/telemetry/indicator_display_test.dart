import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';
import 'package:tccelta_mobile/src/domain/telemetry/indicator_display.dart';

void main() {
  group('IndicatorDisplay.defaultFor', () {
    test('formato padrão é número simples, gauge em anel/pequeno', () {
      final display = IndicatorDisplay.defaultFor(Obd2Pid.rpm);

      expect(display.format, IndicatorFormat.number);
      expect(display.gaugeStyle, IndicatorGaugeStyle.ring);
      expect(display.gaugeSize, IndicatorGaugeSize.small);
      expect(display.historyPoints, HistoryPointsRange.defaultCount);
    });

    test('escala e zonas vêm dos defaults do PID', () {
      final display = IndicatorDisplay.defaultFor(Obd2Pid.rpm);

      expect(display.min, Obd2Pid.rpm.defaultMin);
      expect(display.max, Obd2Pid.rpm.defaultMax);
      expect(display.lowMax, Obd2Pid.rpm.defaultLowMax);
      expect(display.highMin, Obd2Pid.rpm.defaultHighMin);
    });

    test('cada PID tem defaults fisicamente coerentes (min < baixo até < '
        'médio até < máx)', () {
      for (final pid in Obd2Pid.values) {
        expect(
          pid.defaultMin,
          lessThan(pid.defaultLowMax),
          reason: '${pid.name}: min deveria ser menor que "baixo até"',
        );
        expect(
          pid.defaultLowMax,
          lessThan(pid.defaultHighMin),
          reason: '${pid.name}: "baixo até" deveria ser menor que "médio até"',
        );
        expect(
          pid.defaultHighMin,
          lessThanOrEqualTo(pid.defaultMax),
          reason: '${pid.name}: "médio até" não deveria passar do máximo',
        );
      }
    });
  });

  group('IndicatorDisplay — copyWith', () {
    test('sobrescreve só os campos informados', () {
      final base = IndicatorDisplay.defaultFor(Obd2Pid.rpm);

      final gauge = base.copyWith(
        format: IndicatorFormat.gauge,
        gaugeStyle: IndicatorGaugeStyle.needle,
      );

      expect(gauge.format, IndicatorFormat.gauge);
      expect(gauge.gaugeStyle, IndicatorGaugeStyle.needle);
      // Demais campos preservados do original.
      expect(gauge.min, base.min);
      expect(gauge.max, base.max);
      expect(gauge.gaugeSize, base.gaugeSize);
      expect(gauge.historyPoints, base.historyPoints);
    });
  });

  group('IndicatorDisplay — serialização', () {
    test('toJson/fromJson faz round-trip sem perda', () {
      const original = IndicatorDisplay(
        format: IndicatorFormat.gauge,
        min: 0,
        max: 8000,
        lowMax: 5000,
        highMin: 6500,
        gaugeStyle: IndicatorGaugeStyle.needle,
        gaugeSize: IndicatorGaugeSize.large,
        historyPoints: 30,
      );

      final restored = IndicatorDisplay.fromJson(original.toJson());

      expect(restored, original);
    });

    test('toJson serializa enums como texto (pronto para JSON de verdade)', () {
      const display = IndicatorDisplay(
        format: IndicatorFormat.history,
        min: 0,
        max: 100,
        lowMax: 60,
        highMin: 85,
      );

      final json = display.toJson();

      expect(json['format'], 'history');
      expect(json['gaugeStyle'], 'ring');
      expect(json['gaugeSize'], 'small');
      expect(json['min'], isA<num>());
      expect(json['historyPoints'], isA<int>());
    });

    test('== e hashCode consideram todos os campos', () {
      final a = IndicatorDisplay.defaultFor(Obd2Pid.rpm);
      final b = IndicatorDisplay.defaultFor(Obd2Pid.rpm);
      final c = a.copyWith(max: a.max + 1);

      expect(a, b);
      expect(a.hashCode, b.hashCode);
      expect(a, isNot(c));
    });
  });
}
