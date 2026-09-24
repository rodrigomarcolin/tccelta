import 'package:tccelta_mobile/src/domain/repositories/dongle_repository.dart';
import 'package:tccelta_mobile/src/domain/repositories/last_dongle_repository.dart';

/// Esquece o dongle atual: encerra a conexão ativa e apaga o registro
/// persistido usado para reconectar sozinho ao abrir o app.
///
/// Orquestra dois repositories (`DongleRepository` + `LastDongleRepository`),
/// que não se conhecem entre si — por isso vive em `application/` em vez de
/// um view_model falar com os dois diretamente (regra do `CLAUDE.md` §5.1).
///
/// Independente das configurações de criptografia: PSK e modo de segurança
/// (`SettingsService`) não são tocados aqui de propósito.
class ForgetDongleUseCase {
  /// Cria o use case sobre os repositories de conexão e do dongle salvo.
  ForgetDongleUseCase(this._dongle, this._lastDongle);

  final DongleRepository _dongle;
  final LastDongleRepository _lastDongle;

  /// Desconecta e apaga o dongle salvo.
  Future<void> call() async {
    await _dongle.disconnect();
    await _lastDongle.clear();
  }
}
