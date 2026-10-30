import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_link_error.dart';

/// Natureza do sinal de falha, já extraída pelo adapter (sem tipos da lib).
enum LinkFailureKind {
  /// `createBond` terminou com o bond em `none` (inclui o *Pairing Failed* do
  /// dongle fora da janela).
  createBondFailed,

  /// `createBond` estourou o prazo.
  createBondTimeout,

  /// O `connect` não completou no prazo.
  connectTimeout,

  /// Erro GATT do Android (`status` cru) numa operação.
  gattStatus,

  /// Erro do CoreBluetooth (iOS), `CBError.code`.
  coreBluetoothError,

  /// Erro ATT do iOS (`CBATTError.code`).
  attError,

  /// A conexão caiu; classificar pelo código de desconexão.
  disconnected,

  /// Qualquer outro.
  other,
}

/// Classifica a falha de enlace em um [BleLinkError] (tabela da spec fase-2/04
/// §5.3). Função pura: o adapter extrai plataforma, etapa e códigos da lib e
/// chama aqui; ajustar os códigos observados em bancada mexe só neste arquivo.
///
/// - [stage]: fase em que a falha ocorreu.
/// - [code]: código do erro (GATT/CoreBluetooth/ATT conforme [kind]).
/// - [disconnectCode]: código da última queda (`disconnectReason.code`).
BleLinkError classifyLinkError({
  required bool isAndroid,
  required BleConnectionPhase stage,
  required LinkFailureKind kind,
  int? code,
  int? disconnectCode,
}) {
  switch (kind) {
    case LinkFailureKind.createBondFailed:
      return BleLinkError.pairingRejected;
    case LinkFailureKind.createBondTimeout:
      return BleLinkError.pairingTimeout;
    case LinkFailureKind.connectTimeout:
      return BleLinkError.timeout;
    case LinkFailureKind.disconnected:
      if (disconnectCode == null) return BleLinkError.other;
      return _fromDisconnect(isAndroid, stage, disconnectCode);
    case LinkFailureKind.gattStatus:
      if (isAndroid && (code == 5 || code == 15 || code == 137)) {
        return BleLinkError.notAuthorized;
      }
      return BleLinkError.other;
    case LinkFailureKind.coreBluetoothError:
      if (code == 14) return BleLinkError.bondLostIos;
      if (code == 15) return BleLinkError.pairingTimeout;
      return BleLinkError.other;
    case LinkFailureKind.attError:
      if (code == 5 || code == 15) return BleLinkError.notAuthorized;
      return BleLinkError.other;
    case LinkFailureKind.other:
      return BleLinkError.other;
  }
}

BleLinkError _fromDisconnect(
  bool isAndroid,
  BleConnectionPhase stage,
  int code,
) {
  if (isAndroid) {
    switch (code) {
      case 5:
        return BleLinkError.notAuthorized;
      case 6:
      case 61:
        return BleLinkError.bondLost;
      case 9:
        return BleLinkError.busy;
      case 19:
        final early =
            stage == BleConnectionPhase.connecting ||
            stage == BleConnectionPhase.securing;
        return early ? BleLinkError.remoteReset : BleLinkError.other;
    }
    return BleLinkError.other;
  }
  // iOS: o código é o `error.code` do CoreBluetooth, sem o motivo HCI.
  switch (code) {
    case 14:
      return BleLinkError.bondLostIos;
    case 15:
      return BleLinkError.pairingTimeout;
    case 7:
      final securing =
          stage == BleConnectionPhase.securing ||
          stage == BleConnectionPhase.discovering ||
          stage == BleConnectionPhase.enablingNotify;
      return securing ? BleLinkError.notAuthorized : BleLinkError.other;
  }
  return BleLinkError.other;
}
