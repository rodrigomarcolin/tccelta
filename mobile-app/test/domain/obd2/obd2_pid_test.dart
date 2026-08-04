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

  group('Obd2Pid.decode — invariantes', () {
    test('lança AppException sem byte A', () {
      expect(() => Obd2Pid.speed.decode([]), throwsA(isA<AppException>()));
    });

    test('lança AppException sem byte B para RPM', () {
      expect(() => Obd2Pid.rpm.decode([0x17]), throwsA(isA<AppException>()));
    });
  });
}
