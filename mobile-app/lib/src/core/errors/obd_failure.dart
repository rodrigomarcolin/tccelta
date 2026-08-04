import 'package:tccelta_mobile/src/core/errors/failure.dart';

/// Falha ao executar um comando OBD-II/ELM327 sobre a conexão BLE.
///
/// Cobre: ausência de conexão pronta, timeout (comando descartado pelo
/// back-pressure da fila do firmware), resposta `NO DATA`/`?`, ou resposta
/// malformada que não bate com o cabeçalho esperado.
class ObdCommandFailure extends Failure {
  /// Cria a falha com [message] e a [cause] crua opcional.
  const ObdCommandFailure(super.message, {this.cause});

  /// Erro cru que originou a falha (para logging/diagnóstico). @default null
  final Object? cause;
}
