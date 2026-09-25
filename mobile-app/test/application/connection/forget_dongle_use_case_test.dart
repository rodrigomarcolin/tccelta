import 'package:flutter_test/flutter_test.dart';
import 'package:mocktail/mocktail.dart';
import 'package:tccelta_mobile/src/application/connection/forget_dongle_use_case.dart';
import 'package:tccelta_mobile/src/domain/repositories/dongle_repository.dart';
import 'package:tccelta_mobile/src/domain/repositories/last_dongle_repository.dart';

class _MockDongleRepository extends Mock implements DongleRepository {}

class _MockLastDongleRepository extends Mock implements LastDongleRepository {}

void main() {
  late _MockDongleRepository dongle;
  late _MockLastDongleRepository lastDongle;
  late ForgetDongleUseCase useCase;

  setUp(() {
    dongle = _MockDongleRepository();
    lastDongle = _MockLastDongleRepository();
    useCase = ForgetDongleUseCase(dongle, lastDongle);
    when(dongle.disconnect).thenAnswer((_) async {});
    when(lastDongle.clear).thenAnswer((_) async {});
  });

  test('desconecta e apaga o dongle salvo (só os dois repositories '
      'recebidos — não toca em configurações de criptografia)', () async {
    await useCase.call();

    verify(dongle.disconnect).called(1);
    verify(lastDongle.clear).called(1);
    verifyNoMoreInteractions(dongle);
    verifyNoMoreInteractions(lastDongle);
  });
}
