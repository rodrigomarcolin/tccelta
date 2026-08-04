/// Estado do adaptador Bluetooth do telefone, reduzido ao que a UI precisa.
///
/// A lib expõe mais estados (turningOn, unauthorized, etc.); o adapter os
/// colapsa nestes três — só interessa saber se dá para escanear ([on]) ou não.
enum BleAdapterState {
  /// Estado indeterminado (ligando, desligando, sem permissão, indisponível).
  unknown,

  /// Bluetooth desligado.
  off,

  /// Bluetooth ligado e pronto para escanear/conectar.
  on,
}
