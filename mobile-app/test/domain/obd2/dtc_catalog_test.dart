import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_catalog.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_component.dart';

void main() {
  test('dtcCatalog não é vazio e tem códigos únicos', () {
    expect(dtcCatalog, isNotEmpty);

    final codes = dtcCatalog.map((d) => d.code).toSet();
    expect(codes.length, dtcCatalog.length);
  });

  test('dtcCatalog cobre mais de um componente', () {
    final components = dtcCatalog.map((d) => d.component).toSet();
    expect(components.length, greaterThan(1));
    expect(DtcComponent.values.toSet().containsAll(components), isTrue);
  });
}
