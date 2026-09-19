import 'package:tccelta_mobile/src/core/errors/failure.dart';

/// Falha ao ler os códigos de diagnóstico (DTCs) do veículo.
///
/// Cobre: ausência de conexão pronta, timeout na leitura dos Modos 03/07/0A,
/// ou resposta malformada.
class DtcReadFailure extends Failure {
  /// Cria a falha com [message] e a [cause] crua opcional.
  const DtcReadFailure(super.message, {this.cause});

  /// Erro cru que originou a falha (para logging/diagnóstico). @default null
  final Object? cause;
}
