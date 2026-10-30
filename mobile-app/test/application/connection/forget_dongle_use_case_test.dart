import 'package:flutter_test/flutter_test.dart';
import 'package:mocktail/mocktail.dart';
import 'package:tccelta_mobile/src/application/connection/forget_dongle_use_case.dart';
import 'package:tccelta_mobile/src/domain/ble/last_dongle.dart';
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
    when(
      lastDongle.load,
    ).thenAnswer((_) async => const LastDongle(id: '1', name: 'D'));
  });

  test('Android: desconecta, remove o bond e apaga o registro', () async {
    when(() => dongle.canManageBonds).thenReturn(true);
    when(() => dongle.removeBond('1')).thenAnswer((_) async {});

    final result = await useCase();

    expect(result.bondRemoved, isTrue);
    expect(result.needsManualForget, isFalse);
    verify(dongle.disconnect).called(1);
    verify(() => dongle.removeBond('1')).called(1);
    verify(lastDongle.clear).called(1);
  });

  test('falha ao remover o bond não impede limpar o registro', () async {
    when(() => dongle.canManageBonds).thenReturn(true);
    when(() => dongle.removeBond('1')).thenThrow(Exception('bloqueado'));

    final result = await useCase();

    expect(result.bondRemoved, isFalse);
    verify(lastDongle.clear).called(1);
  });

  test('iOS: pede para esquecer manualmente nos Ajustes', () async {
    when(() => dongle.canManageBonds).thenReturn(false);

    final result = await useCase();

    expect(result.bondRemoved, isFalse);
    expect(result.needsManualForget, isTrue);
    verifyNever(() => dongle.removeBond(any()));
    verify(lastDongle.clear).called(1);
  });

  test('sem dongle salvo não há o que esquecer no SO', () async {
    when(lastDongle.load).thenAnswer((_) async => null);
    when(() => dongle.canManageBonds).thenReturn(false);

    final result = await useCase();

    expect(result.needsManualForget, isFalse);
    verify(dongle.disconnect).called(1);
  });
}
