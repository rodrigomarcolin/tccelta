import 'package:tccelta_mobile/src/domain/location/gps_fix.dart';

/// Posição do celular: permissão e fluxo de leituras.
abstract interface class LocationRepository {
  /// Estado atual da permissão, sem abrir diálogo.
  Future<LocationPermissionState> permission();

  /// Pede a permissão ao usuário. Com `grantedWhileInUse`, uma segunda chamada
  /// pede o acesso em segundo plano (ajustes do sistema no Android 11+).
  Future<LocationPermissionState> requestPermission();

  /// Fluxo de posições a cada [interval]. Com [background], mantém o
  /// rastreamento com o app em segundo plano (serviço em primeiro plano no
  /// Android). Termina ao cancelar a assinatura.
  Stream<GpsFix> fixes({required Duration interval, bool background = false});
}
