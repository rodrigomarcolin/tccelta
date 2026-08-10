/// Caminhos das rotas do app.
///
/// Mantidos num arquivo só de constantes (sem dependências de UI) para que as
/// telas possam navegar (`context.go(AppRoutes.scan)`) sem importar o
/// `GoRouter` e criar ciclo de imports.
abstract final class AppRoutes {
  /// Pedir permissões de Bluetooth (rota inicial).
  static const String permissions = '/permissions';

  /// Bluetooth do telefone desligado.
  static const String bluetoothOff = '/bluetooth-off';

  /// Procurar dongles próximos.
  static const String scan = '/scan';

  /// Handshake de conexão em andamento.
  static const String connecting = '/connecting';

  /// Conectado ao dongle.
  static const String connected = '/connected';

  /// Conexão perdida / queda do link.
  static const String connectionLost = '/connection-lost';

  /// Painel de telemetria OBD-II ao vivo (`PainelScreen`).
  static const String painel = '/painel';

  /// Tela de configuração de criptografia (PSK).
  static const String settings = '/settings';
}
