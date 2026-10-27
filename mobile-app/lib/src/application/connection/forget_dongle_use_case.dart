import 'package:tccelta_mobile/src/domain/repositories/dongle_repository.dart';
import 'package:tccelta_mobile/src/domain/repositories/last_dongle_repository.dart';

/// Resultado de esquecer o dongle.
class ForgetDongleResult {
  /// Cria o resultado.
  const ForgetDongleResult({
    required this.bondRemoved,
    required this.needsManualForget,
  });

  /// O bond do SO foi removido pelo app (Android).
  final bool bondRemoved;

  /// O SO não deixa o app remover o bond (iOS): o usuário precisa tocar em
  /// "Esquecer este dispositivo" nos Ajustes do sistema.
  final bool needsManualForget;
}

/// Esquece o dongle atual: encerra a conexão ativa, remove o bond do SO quando
/// possível e apaga o registro persistido usado para reconectar sozinho ao
/// abrir o app.
///
/// Orquestra dois repositories (`DongleRepository` + `LastDongleRepository`),
/// que não se conhecem entre si — por isso vive em `application/` em vez de
/// um view_model falar com os dois diretamente (regra do `CLAUDE.md` §5.1).
class ForgetDongleUseCase {
  /// Cria o use case sobre os repositories de conexão e do dongle salvo.
  ForgetDongleUseCase(this._dongle, this._lastDongle);

  final DongleRepository _dongle;
  final LastDongleRepository _lastDongle;

  /// Desconecta, remove o bond (se o SO permitir) e apaga o dongle salvo.
  Future<ForgetDongleResult> call() async {
    final last = await _lastDongle.load();
    await _dongle.disconnect();
    var bondRemoved = false;
    var needsManualForget = false;
    if (last != null) {
      if (_dongle.canManageBonds) {
        try {
          await _dongle.removeBond(last.id);
          bondRemoved = true;
        } on Object {
          // Falha ao remover o bond não impede esquecer o registro.
        }
      } else {
        needsManualForget = true;
      }
    }
    await _lastDongle.clear();
    return ForgetDongleResult(
      bondRemoved: bondRemoved,
      needsManualForget: needsManualForget,
    );
  }
}
