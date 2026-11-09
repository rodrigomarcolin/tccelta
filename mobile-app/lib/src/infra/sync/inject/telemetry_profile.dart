/// Perfil de motorista de um CSV de telemetria de bancada, usado só na
/// demonstração de segurança (injeção de dados sob o perfil `none`).
enum TelemetryProfile {
  /// Condução mansa e econômica (rotação baixa, consumo baixo).
  economico(
    assetPath: 'assets/benchmark/motorista_economico.csv',
    label: 'Econômico',
  ),

  /// Condução veloz e gastona (rotação alta, consumo alto).
  agressivo(
    assetPath: 'assets/benchmark/motorista_agressivo.csv',
    label: 'Agressivo',
  );

  const TelemetryProfile({required this.assetPath, required this.label});

  /// Caminho do asset CSV com a sequência de amostras.
  final String assetPath;

  /// Rótulo legível (pt-BR) para a UI de depuração.
  final String label;
}
