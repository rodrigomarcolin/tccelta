import 'package:tccelta_mobile/src/domain/telemetry/panel_collection.dart';

/// Contrato de persistência dos painéis do usuário (interface no domain,
/// impl no data).
///
/// É a *source of truth* dos painéis salvos para a camada `application`:
/// indiferente a a implementação usar armazenamento local (hoje,
/// `shared_preferences`) ou um backend remoto no futuro — só a impl do
/// `data` muda, a interface e quem a consome não.
abstract interface class PanelRepository {
  /// Carrega os painéis salvos. `null` quando nada foi persistido ainda
  /// (primeira execução) — não é uma condição de erro.
  Future<PanelsState?> load();

  /// Persiste [state], substituindo o que estava salvo antes.
  Future<void> save(PanelsState state);
}
