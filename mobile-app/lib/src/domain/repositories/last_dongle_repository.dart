import 'package:tccelta_mobile/src/domain/ble/last_dongle.dart';

/// Contrato de persistência do último dongle conectado com sucesso (interface
/// no domain, impl no data).
///
/// Fonte única da verdade para "reconectar sozinho ao abrir o app": indiferente
/// à implementação usar armazenamento local (hoje, `shared_preferences`) ou
/// outra estratégia no futuro — só a impl do `data` muda.
abstract interface class LastDongleRepository {
  /// Carrega o dongle salvo. `null` quando nenhum handshake foi concluído
  /// ainda, ou depois de [clear] — não é uma condição de erro.
  Future<LastDongle?> load();

  /// Persiste [device], substituindo o que estava salvo antes.
  Future<void> save(LastDongle device);

  /// Apaga o dongle salvo (ex.: "Esquecer dispositivo").
  Future<void> clear();
}
