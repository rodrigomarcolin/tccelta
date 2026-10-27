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

/// Falha ao remover o bond do SO com o dongle (a API oculta do Android pode
/// ser bloqueada em versões novas; nesse caso o usuário remove nos Ajustes).
class BleBondFailure extends Failure {
  /// Cria a falha de bond com [message] e a [cause] original opcional.
  const BleBondFailure(super.message, {this.cause});

  /// Erro cru que originou a falha (para logging/diagnóstico). @default null
  final Object? cause;
}

/// O SO não permite ao app gerenciar bonds (iOS: só nos Ajustes do sistema).
class BleBondUnsupportedFailure extends Failure {
  /// Cria a falha com [message].
  const BleBondUnsupportedFailure(super.message);
}
