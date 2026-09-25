/// Contrato de acesso à permissão de câmera (interface no domain, impl no
/// data).
///
/// É a *source of truth* da permissão de câmera para a camada de UI: o view
/// model consome esta interface, nunca o datasource nem o plugin direto.
abstract interface class CameraPermissionsRepository {
  /// Permissão de câmera já concedida? Não abre o diálogo do sistema.
  Future<bool> hasCameraPermission();

  /// Pede a permissão de câmera (abre o diálogo). `true` se concedida.
  Future<bool> requestCameraPermission();

  /// `true` quando a permissão foi negada permanentemente (só os ajustes do
  /// app resolvem).
  Future<bool> isPermanentlyDenied();
}
