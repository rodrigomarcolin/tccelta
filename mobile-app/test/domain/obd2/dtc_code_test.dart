import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_active_entry.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_code.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_component.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_definition.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_severity.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_status.dart';

void main() {
  const definition = DtcDefinition(
    code: 'P0301',
    component: DtcComponent.engine,
    name: 'Falha de combustão — cilindro 1',
    severity: DtcSeverity.high,
    causes: ['Vela ou bobina do cilindro 1'],
  );

  test(
    'sem entrada ativa, o código sai inativo (status/detectedLabel nulos)',
    () {
      final code = DtcCode.fromDefinition(definition);

      expect(code.code, definition.code);
      expect(code.component, definition.component);
      expect(code.name, definition.name);
      expect(code.severity, definition.severity);
      expect(code.causes, definition.causes);
      expect(code.status, isNull);
      expect(code.detectedLabel, isNull);
      expect(code.isActive, isFalse);
    },
  );

  test('com entrada ativa correspondente, repassa status e detectedLabel', () {
    const active = DtcActiveEntry(
      code: 'P0301',
      status: DtcStatus.confirmed,
      detectedLabel: 'há 2 dias · 3 ciclos',
    );

    final code = DtcCode.fromDefinition(definition, active: active);

    expect(code.status, DtcStatus.confirmed);
    expect(code.detectedLabel, 'há 2 dias · 3 ciclos');
    expect(code.isActive, isTrue);
    // Dados de catálogo continuam vindo da definição, não da entrada ativa.
    expect(code.name, definition.name);
    expect(code.causes, definition.causes);
  });
}
