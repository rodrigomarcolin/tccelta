/// OBD2 Cockpit — registro do conjunto de ícones outline customizado.
///
/// São ícones de traço (1.6–2.0, round caps/joins) desenhados em SVG fiel ao
/// design system. Os arquivos vivem em `assets/icons/` com cor `currentColor`,
/// permitindo que o átomo `AppIcon` os recolora por contexto
/// (ativo = ciano, inativo = neutro, destrutivo = vermelho).
enum AppIconData {
  /// Bluetooth — pareamento BLE.
  bluetooth('bluetooth'),

  /// Localização (pin de mapa).
  local('local'),

  /// Buscar (lupa).
  buscar('buscar'),

  /// Estrela — promoção ao dashboard (preenchida quando ativa).
  estrela('estrela'),

  /// Voltar (chevron para a esquerda).
  voltar('voltar'),

  /// Recarregar / atualizar.
  recarregar('recarregar'),

  /// Painel (alvo) — aba do dashboard.
  painel('painel'),

  /// Sensores (lista) — aba de PIDs.
  sensores('sensores'),

  /// Terminal (prompt) — aba de comandos.
  terminal('terminal'),

  /// Mais (kebab) — aba de opções.
  mais('mais'),

  /// Dongle (plugue OBD).
  dongle('dongle'),

  /// Informação.
  info('info'),

  /// Confirmação (check).
  check('check'),

  /// Chevron (avançar).
  chevron('chevron'),

  /// Sinal (intensidade BLE).
  sinal('sinal'),

  /// Desconectar (plugue removido) — ação destrutiva.
  desconectar('desconectar'),

  /// Cadeado — criptografia/PSK.
  cadeado('cadeado');

  const AppIconData(this._name);

  final String _name;

  /// Caminho do asset SVG correspondente.
  String get asset => 'assets/icons/$_name.svg';
}
