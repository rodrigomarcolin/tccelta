import 'package:tccelta_mobile/src/core/errors/failure.dart';

/// Falha ao ler ou gravar os painéis persistidos (ex.: JSON corrompido, erro
/// do plugin de storage). Cache-miss normal (nada salvo ainda) **não** é
/// [PanelStorageFailure] — `PanelRepository.load()` retorna `null` nesse
/// caso.
class PanelStorageFailure extends Failure {
  /// Cria a falha com uma [message] legível e a [cause] original opcional.
  const PanelStorageFailure(super.message, {this.cause});

  /// Exceção crua que originou a falha, quando disponível.
  final Object? cause;
}
