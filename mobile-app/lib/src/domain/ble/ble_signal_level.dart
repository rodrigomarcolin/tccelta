/// Nível de intensidade do sinal BLE derivado do RSSI.
///
/// Regra de negócio PURA (fonte da verdade da classificação): as camadas acima
/// só a aplicam (o datasource) ou exibem (a UI). Os limites em dBm espelham o
/// que o app mostrava antes inline na tela de busca.
enum BleSignalLevel {
  /// Sinal forte (RSSI perto de zero).
  strong,

  /// Sinal médio.
  medium,

  /// Sinal fraco (RSSI mais negativo).
  weak;

  /// Classifica um [rssi] em dBm (mais próximo de zero = mais forte).
  factory BleSignalLevel.fromRssi(int rssi) {
    if (rssi >= -60) return BleSignalLevel.strong;
    if (rssi >= -75) return BleSignalLevel.medium;
    return BleSignalLevel.weak;
  }
}
