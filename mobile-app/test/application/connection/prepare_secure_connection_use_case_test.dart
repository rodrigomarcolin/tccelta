import 'package:flutter_test/flutter_test.dart';
import 'package:mocktail/mocktail.dart';
import 'package:tccelta_mobile/src/application/connection/prepare_secure_connection_use_case.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_bond_state.dart';
import 'package:tccelta_mobile/src/domain/ble/dongle_pairing_hint.dart';
import 'package:tccelta_mobile/src/domain/ble/last_dongle.dart';
import 'package:tccelta_mobile/src/domain/repositories/dongle_repository.dart';
import 'package:tccelta_mobile/src/domain/repositories/last_dongle_repository.dart';

class _MockDongle extends Mock implements DongleRepository {}

class _MockLast extends Mock implements LastDongleRepository {}

void main() {
  late _MockDongle dongle;
  late _MockLast last;
  late PrepareSecureConnectionUseCase useCase;

  const open = DonglePairingHint(windowOpen: true, hasBond: false);
  const closed = DonglePairingHint(windowOpen: false, hasBond: false);

  setUp(() {
    dongle = _MockDongle();
    last = _MockLast();
    useCase = PrepareSecureConnectionUseCase(dongle, last);
  });

  void android(BleBondState bond) {
    when(() => dongle.canManageBonds).thenReturn(true);
    when(() => dongle.bondState('1')).thenAnswer((_) async => bond);
  }

  void ios({required bool remembered}) {
    when(() => dongle.canManageBonds).thenReturn(false);
    when(() => last.load()).thenAnswer(
      (_) async => remembered ? const LastDongle(id: '1', name: 'D') : null,
    );
  }

  Future<SecureConnectionPlan> plan(DonglePairingHint? hint) =>
      useCase(deviceId: '1', hint: hint);

  group('Android', () {
    test('bonded: restaura, qualquer dica', () async {
      android(BleBondState.bonded);
      expect(await plan(null), SecureConnectionPlan.restore);
      expect(await plan(closed), SecureConnectionPlan.restore);
    });

    test('sem bond e janela aberta: pareia agora', () async {
      android(BleBondState.none);
      expect(await plan(open), SecureConnectionPlan.pairNow);
    });

    test('bonding e janela aberta: pareia agora', () async {
      android(BleBondState.bonding);
      expect(await plan(open), SecureConnectionPlan.pairNow);
    });

    test('sem bond e janela fechada ou sem dica: espera o botão', () async {
      android(BleBondState.none);
      expect(await plan(closed), SecureConnectionPlan.waitForButton);
      expect(await plan(null), SecureConnectionPlan.waitForButton);
    });
  });

  group('iOS', () {
    test('dongle lembrado: restaura, qualquer dica', () async {
      ios(remembered: true);
      expect(await plan(closed), SecureConnectionPlan.restore);
    });

    test('não lembrado e janela aberta: pareia agora', () async {
      ios(remembered: false);
      expect(await plan(open), SecureConnectionPlan.pairNow);
    });

    test('não lembrado e janela fechada ou sem dica: espera o botão', () async {
      ios(remembered: false);
      expect(await plan(closed), SecureConnectionPlan.waitForButton);
      expect(await plan(null), SecureConnectionPlan.waitForButton);
    });
  });
}
