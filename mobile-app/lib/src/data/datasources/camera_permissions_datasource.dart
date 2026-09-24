import 'package:permission_handler/permission_handler.dart';

/// Envelopa o plugin `permission_handler` com o que é específico da câmera —
/// usada só para o scanner de QR code da PSK. Não guarda estado — o
/// repository é quem faz isso.
///
/// Espelha o datasource de permissões de BLE. Diferente do fluxo de BLE,
/// aqui a negação permanente (Android "Não perguntar de novo" / iOS após uma
/// negação) é relevante: sem [isPermanentlyDenied], a tela ficaria pedindo um
/// diálogo do sistema que o SO já parou de mostrar.
class CameraPermissionsDatasource {
  /// Cria o datasource.
  const CameraPermissionsDatasource();

  /// Permissão de câmera já concedida? Lê o `status` atual **sem** abrir o
  /// diálogo do sistema.
  Future<bool> hasCameraPermission() async {
    final status = await Permission.camera.status;
    return status.isGranted || status.isLimited;
  }

  /// Pede acesso à câmera (abre o diálogo do sistema). `true` se concedido.
  Future<bool> requestCameraPermission() async {
    final status = await Permission.camera.request();
    return status.isGranted || status.isLimited;
  }

  /// `true` quando o usuário já negou a ponto do SO não mostrar mais o
  /// diálogo — só os ajustes do app resolvem a partir daqui.
  Future<bool> isPermanentlyDenied() async {
    final status = await Permission.camera.status;
    return status.isPermanentlyDenied;
  }
}
