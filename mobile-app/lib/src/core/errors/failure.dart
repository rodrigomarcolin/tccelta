/// Erro de domínio esperado (falha recuperável de negócio/infra), oposto a um
/// bug de programação.
///
/// É a fronteira de erro que as camadas superiores (view models) tratam: os
/// repositories capturam exceções cruas e as convertem em subclasses de
/// [Failure], para que a UI nunca precise conhecer detalhes de plugins/rede.
abstract class Failure implements Exception {
  /// Cria uma falha com uma [message] legível.
  const Failure(this.message);

  /// Mensagem legível descrevendo a falha.
  final String message;

  @override
  String toString() => '$runtimeType: $message';
}

/// Exceção interna do app para condições inesperadas (invariantes quebradas).
///
/// Diferente de [Failure], não é esperada no fluxo normal — sinaliza um bug.
class AppException implements Exception {
  /// Cria a exceção com uma [message] descritiva.
  const AppException(this.message);

  /// Descrição do problema.
  final String message;

  @override
  String toString() => 'AppException: $message';
}
