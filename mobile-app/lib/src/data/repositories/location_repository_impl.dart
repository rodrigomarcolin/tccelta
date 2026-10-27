import 'package:geolocator/geolocator.dart';
import 'package:tccelta_mobile/src/core/errors/location_failure.dart';
import 'package:tccelta_mobile/src/core/time/monotonic_clock.dart';
import 'package:tccelta_mobile/src/data/datasources/location_datasource.dart';
import 'package:tccelta_mobile/src/domain/location/gps_fix.dart';
import 'package:tccelta_mobile/src/domain/repositories/location_repository.dart';

/// Converte o plugin de localização em [GpsFix] e em falhas de domínio.
class LocationRepositoryImpl implements LocationRepository {
  /// Cria o repository sobre o [_ds].
  LocationRepositoryImpl(this._ds, {MonotonicClock? clock})
    : _clock = clock ?? SystemMonotonicClock();

  final LocationDatasource _ds;
  final MonotonicClock _clock;

  @override
  Future<LocationPermissionState> permission() async {
    try {
      if (!await _ds.isServiceEnabled()) {
        return LocationPermissionState.serviceDisabled;
      }
      return _map(await _ds.checkPermission());
    } on Object catch (e) {
      throw LocationFailure('Falha ao consultar a permissão', cause: e);
    }
  }

  @override
  Future<LocationPermissionState> requestPermission() async {
    try {
      if (!await _ds.isServiceEnabled()) {
        return LocationPermissionState.serviceDisabled;
      }
      return _map(await _ds.requestPermission());
    } on Object catch (e) {
      throw LocationFailure('Falha ao pedir a permissão', cause: e);
    }
  }

  @override
  Stream<GpsFix> fixes({required Duration interval, bool background = false}) =>
      _ds
          .positions(interval: interval, background: background)
          .map(
            (p) => GpsFix.fromDegrees(
              lat: p.latitude,
              lon: p.longitude,
              altM: p.altitude,
              accuracyM: p.accuracy,
              at: _clock.now(),
            ),
          )
          .handleError((Object e) {
            throw LocationFailure('Falha ao ler a posição', cause: e);
          });

  LocationPermissionState _map(LocationPermission p) => switch (p) {
    LocationPermission.always => LocationPermissionState.granted,
    LocationPermission.whileInUse => LocationPermissionState.grantedWhileInUse,
    LocationPermission.deniedForever => LocationPermissionState.deniedForever,
    LocationPermission.denied ||
    LocationPermission.unableToDetermine => LocationPermissionState.denied,
  };
}
