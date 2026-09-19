/// Severidade de um `DtcCode` — orienta a cor/ênfase com que o código é
/// exibido (a UI mapeia cada valor para um tom do design system).
enum DtcSeverity {
  /// Alta — falha com potencial de dano ao veículo ou perda de dirigibilidade.
  high,

  /// Média — falha relevante, sem risco imediato.
  medium,

  /// Baixa — falha de conforto/acessório, sem impacto na condução.
  low,
}
