import 'package:tccelta_mobile/src/core/errors/failure.dart';

/// Falha ao escanear dispositivos BLE (adaptador desligado, permissão negada,
/// erro do plugin de scan).
class BleScanFailure extends Failure {
  /// Cria a falha de scan com [message] e a [cause] original opcional.
  const BleScanFailure(super.message, {this.cause});

  /// Erro cru que originou a falha (para logging/diagnóstico). @default null
  final Object? cause;
}

/// Falha ao conectar ou preparar o link BLE (timeout, serviço/característica
/// ausente, notify recusado, queda durante o handshake).
class BleConnectionFailure extends Failure {
  /// Cria a falha de conexão com [message] e a [cause] original opcional.
  const BleConnectionFailure(super.message, {this.cause});

  /// Erro cru que originou a falha (para logging/diagnóstico). @default null
  final Object? cause;
}
