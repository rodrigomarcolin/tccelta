import 'package:tccelta_mobile/src/core/errors/failure.dart';

/// Falha ao ler ou gravar o último dongle persistido (ex.: JSON corrompido,
/// erro do plugin de storage). Cache-miss normal (nada salvo ainda) **não**
/// é [LastDongleStorageFailure] — `LastDongleRepository.load()` retorna
/// `null` nesse caso.
class LastDongleStorageFailure extends Failure {
  /// Cria a falha com uma [message] legível e a [cause] original opcional.
  const LastDongleStorageFailure(super.message, {this.cause});

  /// Exceção crua que originou a falha, quando disponível.
  final Object? cause;
}
