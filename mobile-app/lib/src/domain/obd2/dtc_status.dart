/// Estado de ativação de um `DtcCode` no veículo.
///
/// Mapeia os modos OBD-II de leitura de códigos: [confirmed] vem do Modo 03
/// (armazenado — já confirmado em 2+ ciclos, acende a luz MIL quando
/// aplicável); [pending] vem do Modo 07 (visto neste ciclo, ainda não
/// confirmado); [permanent] vem do Modo 0A (confirmado que sobrevive a um
/// reset de códigos — só o próprio veículo o apaga, após ciclos limpos). Um
/// `DtcCode` sem `status` (`null`) está no catálogo do veículo mas não está
/// ativo na leitura atual.
enum DtcStatus {
  /// Confirmado (Modo 03) — acende a luz de falha quando presente.
  confirmed,

  /// Pendente (Modo 07) — visto neste ciclo, aguardando confirmação.
  pending,

  /// Permanente (Modo 0A) — confirmado e imune a reset manual de códigos.
  permanent,
}
