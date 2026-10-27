import 'package:tccelta_mobile/src/core/errors/failure.dart';

/// Falha ao gravar as configurações persistidas (erro do plugin de storage).
class SettingsWriteFailure extends Failure {
  /// Cria a falha com [message] e a [cause] original opcional.
  const SettingsWriteFailure(super.message, {this.cause});

  /// Exceção crua que originou a falha, quando disponível.
  final Object? cause;
}
