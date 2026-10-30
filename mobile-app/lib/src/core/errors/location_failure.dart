import 'package:tccelta_mobile/src/core/errors/failure.dart';

/// Falha ao obter a posição ou a permissão de localização.
class LocationFailure extends Failure {
  /// Cria a falha com [message] e a [cause] original opcional.
  const LocationFailure(super.message, {this.cause});

  /// Exceção crua que originou a falha, quando disponível.
  final Object? cause;
}
