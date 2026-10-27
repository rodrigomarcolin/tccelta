/// Estado do bond (pareamento) do SO com um dispositivo BLE.
enum BleBondState {
  /// Sem bond.
  none,

  /// Pareamento em andamento.
  bonding,

  /// Bond existente (a chave de longo prazo está guardada no SO).
  bonded,

  /// O SO não informa (iOS não expõe o estado do bond ao app).
  unknown,
}
