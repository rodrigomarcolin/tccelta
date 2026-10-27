import 'package:tccelta_mobile/src/domain/ble/ble_bond_state.dart';
import 'package:tccelta_mobile/src/domain/ble/dongle_pairing_hint.dart';
import 'package:tccelta_mobile/src/domain/repositories/dongle_repository.dart';
import 'package:tccelta_mobile/src/domain/repositories/last_dongle_repository.dart';

/// Como conectar a um dongle, dado o estado de pareamento conhecido.
enum SecureConnectionPlan {
  /// Há bond: conecta e o SO cifra com a chave guardada.
  restore,

  /// Sem bond e com a janela do dongle aberta: conecta e pareia agora.
  pairNow,

  /// Sem bond e janela fechada: o usuário precisa segurar o botão do dongle.
  waitForButton,
}

/// Decide o plano de conexão segura a um dongle.
///
/// Combina dois repositories (`DongleRepository` para o bond do SO e
/// `LastDongleRepository` para o registro do app), por isso vive em
/// `application/`. A dica do anúncio nunca decide sozinha uma ação
/// destrutiva: ela só escolhe entre parear agora e esperar o botão.
class PrepareSecureConnectionUseCase {
  /// Cria o use case sobre os repositories de conexão e do dongle salvo.
  PrepareSecureConnectionUseCase(this._dongle, this._lastDongle);

  final DongleRepository _dongle;
  final LastDongleRepository _lastDongle;

  /// Decide como conectar a [deviceId]; [hint] vem do anúncio visto na busca.
  Future<SecureConnectionPlan> call({
    required String deviceId,
    DonglePairingHint? hint,
  }) async {
    final windowOpen = hint?.windowOpen ?? false;
    final bool known;
    if (_dongle.canManageBonds) {
      // Android: o SO informa o bond.
      known = await _dongle.bondState(deviceId) == BleBondState.bonded;
    } else {
      // iOS: só o registro do próprio app (gravado após uma sessão autorizada).
      known = (await _lastDongle.load())?.id == deviceId;
    }
    if (known) return SecureConnectionPlan.restore;
    return windowOpen
        ? SecureConnectionPlan.pairNow
        : SecureConnectionPlan.waitForButton;
  }
}
