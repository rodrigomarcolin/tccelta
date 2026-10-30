import 'package:flutter/foundation.dart'
    show TargetPlatform, defaultTargetPlatform;
import 'package:geolocator/geolocator.dart';

/// Envelopa o plugin `geolocator`: única classe do app que o importa.
class LocationDatasource {
  /// Cria o datasource.
  const LocationDatasource();

  /// Permissão atual, sem diálogo.
  Future<LocationPermission> checkPermission() => Geolocator.checkPermission();

  /// Pede a permissão ao usuário.
  Future<LocationPermission> requestPermission() =>
      Geolocator.requestPermission();

  /// `true` com o serviço de localização ligado no aparelho.
  Future<bool> isServiceEnabled() => Geolocator.isLocationServiceEnabled();

  /// Fluxo de posições a cada [interval]. [background] liga o serviço em
  /// primeiro plano (Android) e as atualizações em segundo plano (iOS).
  Stream<Position> positions({
    required Duration interval,
    required bool background,
  }) {
    final settings = _isIos
        ? AppleSettings(
            accuracy: LocationAccuracy.high,
            activityType: ActivityType.automotiveNavigation,
            showBackgroundLocationIndicator: background,
            allowBackgroundLocationUpdates: background,
          )
        : AndroidSettings(
            accuracy: LocationAccuracy.high,
            intervalDuration: interval,
            foregroundNotificationConfig: background
                ? const ForegroundNotificationConfig(
                    notificationTitle: 'OBD2 Cockpit',
                    notificationText: 'Registrando a posição do veículo',
                    enableWakeLock: true,
                  )
                : null,
          );
    return Geolocator.getPositionStream(locationSettings: settings);
  }

  static bool get _isIos => defaultTargetPlatform == TargetPlatform.iOS;
}
