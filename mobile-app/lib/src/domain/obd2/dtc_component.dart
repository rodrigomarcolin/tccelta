/// Componente/sistema do veículo a que um `DtcCode` pertence — o agrupamento
/// usado na vista "Todos por componente" da aba de Diagnóstico.
///
/// Curadoria alinhada ao design (`DTC_GROUPS` do mock): categorias amplas o
/// bastante para cobrir os prefixos padrão de DTC (P = powertrain, B = body,
/// C = chassis, U = network) sem exigir, nesta fase, um mapeamento código→
/// componente vindo do firmware — isso é decisão futura do datasource real.
enum DtcComponent {
  /// Motor — combustão, ignição, mistura.
  engine('Motor', 'combustão, ignição, mistura'),

  /// Emissões — catalisador, EVAP, sondas.
  emissions('Emissões', 'catalisador, EVAP, sondas'),

  /// Transmissão — câmbio e conversor.
  transmission('Transmissão', 'câmbio e conversor'),

  /// Freios & chassi — ABS, sensores de roda.
  brakes('Freios & chassi', 'ABS, sensores de roda'),

  /// Carroceria & conforto — vidros, travas, iluminação.
  body('Carroceria & conforto', 'vidros, travas, iluminação'),

  /// Rede & módulos — comunicação CAN.
  network('Rede & módulos', 'comunicação CAN');

  const DtcComponent(this.label, this.hint);

  /// Nome exibido do componente.
  final String label;

  /// Descrição curta do que esse componente cobre.
  final String hint;
}
