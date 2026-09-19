/// Estado de ativação de um `DtcCode` no veículo.
///
/// Mapeia os modos OBD-II de leitura de códigos: [confirmed] vem do Modo 03
/// (armazenado — já confirmado em 2+ ciclos, acende a luz MIL quando
/// aplicável); [pending] vem do Modo 07 (visto neste ciclo, ainda não
/// confirmado). Um `DtcCode` sem `status` (`null`) está no catálogo do
/// veículo mas não está ativo na leitura atual.
enum DtcStatus {
  /// Confirmado (Modo 03) — acende a luz de injeção quando presente.
  confirmed,

  /// Pendente (Modo 07) — visto neste ciclo, aguardando confirmação.
  pending,
}
