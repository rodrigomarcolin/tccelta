import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_link_error.dart';
import 'package:tccelta_mobile/src/infra/ble/ble_link_error_classifier.dart';

void main() {
  BleLinkError android(
    LinkFailureKind kind, {
    int? code,
    int? disconnectCode,
    BleConnectionPhase stage = BleConnectionPhase.securing,
  }) => classifyLinkError(
    isAndroid: true,
    stage: stage,
    kind: kind,
    code: code,
    disconnectCode: disconnectCode,
  );

  BleLinkError ios(
    LinkFailureKind kind, {
    int? code,
    int? disconnectCode,
    BleConnectionPhase stage = BleConnectionPhase.enablingNotify,
  }) => classifyLinkError(
    isAndroid: false,
    stage: stage,
    kind: kind,
    code: code,
    disconnectCode: disconnectCode,
  );

  group('Android', () {
    test('createBond falhou: recusado', () {
      expect(
        android(LinkFailureKind.createBondFailed),
        BleLinkError.pairingRejected,
      );
    });

    test('createBond estourou o prazo', () {
      expect(
        android(LinkFailureKind.createBondTimeout),
        BleLinkError.pairingTimeout,
      );
    });

    test('queda com código 5, 6, 61 e 9', () {
      expect(
        android(LinkFailureKind.disconnected, disconnectCode: 5),
        BleLinkError.notAuthorized,
      );
      expect(
        android(LinkFailureKind.disconnected, disconnectCode: 6),
        BleLinkError.bondLost,
      );
      expect(
        android(LinkFailureKind.disconnected, disconnectCode: 61),
        BleLinkError.bondLost,
      );
      expect(
        android(LinkFailureKind.disconnected, disconnectCode: 9),
        BleLinkError.busy,
      );
    });

    test('queda 0x13 só é reset durante connecting/securing', () {
      expect(
        android(LinkFailureKind.disconnected, disconnectCode: 19),
        BleLinkError.remoteReset,
      );
      expect(
        android(
          LinkFailureKind.disconnected,
          disconnectCode: 19,
          stage: BleConnectionPhase.discovering,
        ),
        BleLinkError.other,
      );
    });

    test('erro GATT 5, 15 e 137: não autorizado', () {
      for (final code in [5, 15, 137]) {
        expect(
          android(LinkFailureKind.gattStatus, code: code),
          BleLinkError.notAuthorized,
        );
      }
      expect(
        android(LinkFailureKind.gattStatus, code: 133),
        BleLinkError.other,
      );
    });

    test('timeout do connect e demais', () {
      expect(android(LinkFailureKind.connectTimeout), BleLinkError.timeout);
      expect(android(LinkFailureKind.other), BleLinkError.other);
      expect(android(LinkFailureKind.disconnected), BleLinkError.other);
    });
  });

  group('iOS', () {
    test('CBError 14 e 15', () {
      expect(
        ios(LinkFailureKind.coreBluetoothError, code: 14),
        BleLinkError.bondLostIos,
      );
      expect(
        ios(LinkFailureKind.coreBluetoothError, code: 15),
        BleLinkError.pairingTimeout,
      );
    });

    test('erro ATT 5 ou 15: não autorizado', () {
      expect(
        ios(LinkFailureKind.attError, code: 5),
        BleLinkError.notAuthorized,
      );
      expect(
        ios(LinkFailureKind.attError, code: 15),
        BleLinkError.notAuthorized,
      );
    });

    test('queda 7 durante a segurança é ambígua: não autorizado', () {
      expect(
        ios(LinkFailureKind.disconnected, disconnectCode: 7),
        BleLinkError.notAuthorized,
      );
      expect(
        ios(
          LinkFailureKind.disconnected,
          disconnectCode: 7,
          stage: BleConnectionPhase.connecting,
        ),
        BleLinkError.other,
      );
    });

    test('queda com código 14 e timeout do connect', () {
      expect(
        ios(LinkFailureKind.disconnected, disconnectCode: 14),
        BleLinkError.bondLostIos,
      );
      expect(ios(LinkFailureKind.connectTimeout), BleLinkError.timeout);
    });
  });
}
