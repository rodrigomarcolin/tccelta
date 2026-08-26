import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/core/errors/failure.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';

void main() {
  group('Obd2Pid.command', () {
    test('monta o comando ELM327 em hex de 2 dígitos', () {
      expect(Obd2Pid.rpm.command, '010C');
      expect(Obd2Pid.engineLoad.command, '0104');
      expect(Obd2Pid.throttle.command, '0111');
    });
  });

  group('Obd2Pid.shortLabel', () {
    test('todo PID tem um rótulo curto não-vazio para o gauge', () {
      for (final pid in Obd2Pid.values) {
        expect(
          pid.shortLabel,
          isNotEmpty,
          reason: '${pid.name} sem shortLabel',
        );
        expect(
          pid.shortLabel.length,
          lessThanOrEqualTo(14),
          reason: '${pid.name}.shortLabel longo demais para o gauge',
        );
      }
    });
  });

  group('Obd2Pid.decode — valores de exemplo da documentação', () {
    test('carga do motor: A/2.55 (0x66 -> 40%)', () {
      expect(Obd2Pid.engineLoad.decode([0x66]), closeTo(40, 0.5));
    });

    test('temp. do líquido: A-40 (0x82 -> 90°C)', () {
      expect(Obd2Pid.coolantTemp.decode([0x82]), 90);
    });

    test('rotação: ((A*256)+B)/4 (0x17,0x70 -> 1500 RPM)', () {
      expect(Obd2Pid.rpm.decode([0x17, 0x70]), 1500);
    });

    test('velocidade: A (0x3C -> 60 km/h)', () {
      expect(Obd2Pid.speed.decode([0x3C]), 60);
    });

    test('avanço de ignição: A/2-64 (0x94 -> 10°)', () {
      expect(Obd2Pid.timingAdvance.decode([0x94]), 10);
    });

    test('posição do acelerador: A/2.55 (0x33 -> 20%)', () {
      expect(Obd2Pid.throttle.decode([0x33]), closeTo(20, 0.5));
    });
  });

  group('Obd2Pid.decode — PIDs novos (fórmulas de 1 byte)', () {
    test('ajuste de combustível curto prazo B1: 100A/128-100 '
        '(0x86 -> 4,6875%)', () {
      expect(Obd2Pid.shortFuelTrim1.decode([0x86]), 4.6875);
    });

    test('ajuste de combustível longo prazo B1: 100A/128-100 '
        '(0x86 -> 4,6875%)', () {
      expect(Obd2Pid.longFuelTrim1.decode([0x86]), 4.6875);
    });

    test('pressão de combustível: 3A (0x85 -> 399 kPa)', () {
      expect(Obd2Pid.fuelPressureGauge.decode([0x85]), 399);
    });

    test('pressão absoluta do coletor de admissão: A (0x78 -> 120 kPa)', () {
      expect(Obd2Pid.intakeManifoldPressure.decode([0x78]), 120);
    });

    test('temp. do ar de admissão: A-40 (0x5A -> 50°C)', () {
      expect(Obd2Pid.intakeAirTemp.decode([0x5A]), 50);
    });

    test('sonda lambda 1: A/200 (0x8C -> 0,7V)', () {
      expect(Obd2Pid.o2Sensor1Voltage.decode([0x8C]), 0.7);
    });

    test('sonda lambda 2 pós-catalisador: A/200 (0x8C -> 0,7V)', () {
      expect(Obd2Pid.o2Sensor2Voltage.decode([0x8C]), 0.7);
    });

    test('EGR comandada: 100A/255 (0x8C -> ~54,9%)', () {
      expect(Obd2Pid.commandedEgr.decode([0x8C]), closeTo(54.9, 0.1));
    });

    test('erro de EGR: 100A/128-100 (0x84 -> 3,125%)', () {
      expect(Obd2Pid.egrError.decode([0x84]), 3.125);
    });

    test('purga evaporativa comandada: 100A/255 (0xA6 -> ~65,1%)', () {
      expect(Obd2Pid.commandedEvapPurge.decode([0xA6]), closeTo(65.1, 0.1));
    });

    test('nível do tanque de combustível: 100A/255 (0x4D -> ~30,2%)', () {
      expect(Obd2Pid.fuelTankLevel.decode([0x4D]), closeTo(30.2, 0.1));
    });

    test('ciclos de aquecimento desde reset: A (0x23 -> 35)', () {
      expect(Obd2Pid.warmupsSinceClear.decode([0x23]), 35);
    });

    test('pressão barométrica absoluta: A (0x64 -> 100 kPa)', () {
      expect(Obd2Pid.baroPressure.decode([0x64]), 100);
    });

    test('posição relativa do acelerador: 100A/255 (0x8C -> ~54,9%)', () {
      expect(Obd2Pid.relativeThrottle.decode([0x8C]), closeTo(54.9, 0.1));
    });

    test('temp. do ar ambiente: A-40 (0x4E -> 38°C)', () {
      expect(Obd2Pid.ambientAirTemp.decode([0x4E]), 38);
    });

    test('posição da borboleta B: 100A/255 (0x8C -> ~54,9%)', () {
      expect(Obd2Pid.throttlePositionB.decode([0x8C]), closeTo(54.9, 0.1));
    });

    test('posição do pedal do acelerador D: 100A/255 (0x8C -> ~54,9%)', () {
      expect(Obd2Pid.acceleratorPedalD.decode([0x8C]), closeTo(54.9, 0.1));
    });

    test('atuador de borboleta comandado: 100A/255 (0x8C -> ~54,9%)', () {
      expect(
        Obd2Pid.commandedThrottleActuator.decode([0x8C]),
        closeTo(54.9, 0.1),
      );
    });

    test('percentual de etanol: 100A/255 (0x45 -> ~27,1%)', () {
      expect(Obd2Pid.ethanolPercent.decode([0x45]), closeTo(27.1, 0.1));
    });

    test('posição relativa do pedal do acelerador: 100A/255 '
        '(0x8C -> ~54,9%)', () {
      expect(
        Obd2Pid.relativeAcceleratorPedal.decode([0x8C]),
        closeTo(54.9, 0.1),
      );
    });

    test('temp. do óleo do motor: A-40 (0x9B -> 115°C)', () {
      expect(Obd2Pid.engineOilTemp.decode([0x9B]), 115);
    });

    test('torque demandado pelo motorista: A-125 (0xB9 -> 60%)', () {
      expect(Obd2Pid.driverDemandTorque.decode([0xB9]), 60);
    });

    test('torque real do motor: A-125 (0xB4 -> 55%)', () {
      expect(Obd2Pid.actualEngineTorque.decode([0xB4]), 55);
    });
  });

  group('Obd2Pid.decode — PIDs novos (fórmulas de 2 bytes)', () {
    test('MAF: (256A+B)/100 (0x0F,0xA0 -> 40 g/s)', () {
      expect(Obd2Pid.maf.decode([0x0F, 0xA0]), 40);
    });

    test('tempo de funcionamento do motor: 256A+B (0x09,0x60 -> 2400s)', () {
      expect(Obd2Pid.engineRunTime.decode([0x09, 0x60]), 2400);
    });

    test('distância com MIL acesa: 256A+B (0x00,0x23 -> 35km)', () {
      expect(Obd2Pid.distanceWithMil.decode([0x00, 0x23]), 35);
    });

    test('distância desde reset de códigos: 256A+B (0x02,0xEE -> 750km)', () {
      expect(Obd2Pid.distanceSinceClear.decode([0x02, 0xEE]), 750);
    });

    test('pressão de vapor evaporativo: (256A+B)/4 (0x00,0xDC -> 55Pa)', () {
      expect(Obd2Pid.evapVaporPressure.decode([0x00, 0xDC]), 55);
    });

    test('temp. do catalisador: (256A+B)/10-40 (0x19,0x00 -> 600°C)', () {
      expect(Obd2Pid.catalystTemp1.decode([0x19, 0x00]), 600);
    });

    test('tensão do módulo de controle: (256A+B)/1000 '
        '(0x35,0xE8 -> 13,8V)', () {
      expect(Obd2Pid.controlModuleVoltage.decode([0x35, 0xE8]), 13.8);
    });

    test('carga absoluta do motor: 100(256A+B)/255 '
        '(0x00,0xE6 -> ~90,2%)', () {
      expect(Obd2Pid.absoluteLoad.decode([0x00, 0xE6]), closeTo(90.2, 0.1));
    });

    test('razão ar-combustível comandada: 2(256A+B)/65536 '
        '(0x80,0x00 -> 1,0λ)', () {
      expect(Obd2Pid.commandedEquivRatio.decode([0x80, 0x00]), 1.0);
    });

    test('tempo com MIL acesa: 256A+B (0x00,0xC8 -> 200min)', () {
      expect(Obd2Pid.timeMilOn.decode([0x00, 0xC8]), 200);
    });

    test('tempo desde reset de códigos: 256A+B (0x02,0xEE -> 750min)', () {
      expect(Obd2Pid.timeSinceClear.decode([0x02, 0xEE]), 750);
    });

    test('temporização de injeção: (256A+B)/128-210 '
        '(0x75,0x80 -> 25°)', () {
      expect(Obd2Pid.fuelInjectionTiming.decode([0x75, 0x80]), 25);
    });

    test('taxa de consumo de combustível: (256A+B)/20 '
        '(0x01,0xF4 -> 25 L/h)', () {
      expect(Obd2Pid.engineFuelRate.decode([0x01, 0xF4]), 25);
    });

    test('torque de referência do motor: 256A+B (0x01,0xC2 -> 450 N·m)', () {
      expect(Obd2Pid.engineReferenceTorque.decode([0x01, 0xC2]), 450);
    });
  });

  group('Obd2Pid.decode — invariantes', () {
    test('lança AppException sem byte A', () {
      expect(() => Obd2Pid.speed.decode([]), throwsA(isA<AppException>()));
    });

    test('lança AppException sem byte B para RPM', () {
      expect(() => Obd2Pid.rpm.decode([0x17]), throwsA(isA<AppException>()));
    });
  });
}
