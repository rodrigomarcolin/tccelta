import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';

void main() {
  group('Obd2Pid.encode (inverso de decode)', () {
    final valores = <Obd2Pid, List<double>>{
      Obd2Pid.rpm: [800, 1970, 3500, 6500],
      Obd2Pid.speed: [0, 45, 120, 180],
      Obd2Pid.engineLoad: [15, 30, 75, 100],
      Obd2Pid.maf: [2.5, 9, 25, 55],
      Obd2Pid.acceleratorPedalD: [9, 30, 70, 100],
      Obd2Pid.engineFuelRate: [0.8, 3.2, 18, 38],
      Obd2Pid.coolantTemp: [85, 90, 100, 108],
      Obd2Pid.fuelTankLevel: [20, 52, 62],
    };
    // Tolerância = passo de quantização do byte de cada PID.
    final tolerancia = <Obd2Pid, double>{
      Obd2Pid.rpm: 0.3,
      Obd2Pid.speed: 0.5,
      Obd2Pid.engineLoad: 0.5,
      Obd2Pid.maf: 0.01,
      Obd2Pid.acceleratorPedalD: 0.5,
      Obd2Pid.engineFuelRate: 0.05,
      Obd2Pid.coolantTemp: 0.5,
      Obd2Pid.fuelTankLevel: 0.5,
    };

    valores.forEach((pid, lista) {
      for (final v in lista) {
        test('${pid.name} ida e volta em $v', () {
          final back = pid.decode(pid.encode(v));
          expect((back - v).abs(), lessThanOrEqualTo(tolerancia[pid]!));
        });
      }
    });

    test('bytes cabem em 0…255', () {
      valores.forEach((pid, lista) {
        for (final v in lista) {
          for (final b in pid.encode(v)) {
            expect(b, inInclusiveRange(0, 255));
          }
        }
      });
    });

    test('PID fora do conjunto lança UnsupportedError', () {
      expect(() => Obd2Pid.timingAdvance.encode(10), throwsUnsupportedError);
    });
  });
}
