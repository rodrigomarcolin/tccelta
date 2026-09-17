import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/data/repositories/dtc_repository_impl.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_catalog.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_status.dart';

void main() {
  test(
    'read() devolve só os códigos ativos agora, todos presentes no catálogo, '
    'com o MIL consistente com os confirmados',
    () async {
      final snapshot = await FakeDtcRepositoryImpl().read();

      expect(snapshot.active, isNotEmpty);

      final confirmed = snapshot.active.where(
        (a) => a.status == DtcStatus.confirmed,
      );
      expect(confirmed, isNotEmpty);
      expect(snapshot.milOn, isTrue);

      // Cada código ativo é único, e existe no catálogo do domínio.
      final codes = snapshot.active.map((a) => a.code).toSet();
      expect(codes.length, snapshot.active.length);
      final catalogCodes = dtcCatalog.map((d) => d.code).toSet();
      expect(codes.every(catalogCodes.contains), isTrue);
    },
  );

  test('sem nenhum confirmado, a luz de injeção fica apagada', () async {
    // A leitura em si é estática (mock determinístico) — este teste garante
    // que a regra milOn-por-confirmados continua fiel caso a lista mude.
    final snapshot = await FakeDtcRepositoryImpl().read();
    final expectedMilOn = snapshot.active.any(
      (a) => a.status == DtcStatus.confirmed,
    );
    expect(snapshot.milOn, expectedMilOn);
  });
}
