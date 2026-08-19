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

  /// Tela de opções ("Mais"), aberta pela aba homônima da tab bar.
  static const String more = '/more';

  /// Configuração da PSK dentro do fluxo de conexão (antes de conectar).
  static const String pskSetup = '/psk-setup';

  /// Lista de sensores (`SensorPickerScreen`). Dois modos de entrada: o
  /// botão "Adicionar indicador" do Painel empilha com `context.push` (sem
  /// `extra`); a aba "Sensores" da tab bar navega com
  /// `context.go(sensorPicker, extra: true)`, tornando a tela raiz dessa
  /// aba. A escolha de painel/formato do indicador são bottom sheets
  /// (`showPanelPickerSheet`/`showIndicatorFormatSheet`), não rotas.
  static const String sensorPicker = '/sensor-picker';
}
