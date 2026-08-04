/// Contrato de acesso às permissões de Bluetooth (interface no domain, impl no
/// data).
///
/// É a *source of truth* das permissões para a camada de UI: o view model
/// consome esta interface, nunca o datasource nem o plugin direto.
abstract interface class PermissionsRepository {
  /// Permissão de Bluetooth já concedida? Não abre o diálogo do sistema.
  Future<bool> hasBluetoothPermission();

  /// Pede a permissão de Bluetooth (abre o diálogo). `true` se concedida.
  Future<bool> requestBluetoothPermission();
}
