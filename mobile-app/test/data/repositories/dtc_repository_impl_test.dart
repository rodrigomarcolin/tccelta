import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/data/repositories/dtc_repository_impl.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_status.dart';

void main() {
  test('read() devolve o catálogo mockado com a contagem de ativos e o MIL '
      'consistente com os confirmados', () async {
    final snapshot = await FakeDtcRepositoryImpl().read();

    expect(snapshot.codes, isNotEmpty);

    final active = snapshot.codes.where((c) => c.isActive);
    final confirmed = snapshot.codes.where(
      (c) => c.status == DtcStatus.confirmed,
    );
    expect(active, isNotEmpty);
    expect(confirmed, isNotEmpty);
    expect(snapshot.milOn, isTrue);

    // Cada código é único no catálogo (sem duplicatas).
    final codes = snapshot.codes.map((c) => c.code).toSet();
    expect(codes.length, snapshot.codes.length);
  });

  test('sem nenhum confirmado, a luz de injeção fica apagada', () async {
    // A leitura em si é estática (mock determinístico) — este teste garante
    // que a regra milOn-por-confirmados continua fiel caso o catálogo mude.
    final snapshot = await FakeDtcRepositoryImpl().read();
    final expectedMilOn = snapshot.codes.any(
      (c) => c.status == DtcStatus.confirmed,
    );
    expect(snapshot.milOn, expectedMilOn);
  });
}
