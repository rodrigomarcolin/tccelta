import 'package:tccelta_mobile/src/core/errors/location_failure.dart';
import 'package:tccelta_mobile/src/core/time/monotonic_clock.dart';
import 'package:tccelta_mobile/src/data/datasources/simulated_gps_datasource.dart';
import 'package:tccelta_mobile/src/domain/location/gps_fix.dart';
import 'package:tccelta_mobile/src/domain/location/simulated_gps_payload.dart';
import 'package:tccelta_mobile/src/domain/repositories/location_repository.dart';

/// [LocationRepository] alimentado pelo simulador de GPS via BLE. Não depende
/// de permissão de localização do aparelho.
class SimulatedLocationRepositoryImpl implements LocationRepository {
  /// Cria o repository sobre o [_ds].
  SimulatedLocationRepositoryImpl(this._ds, {MonotonicClock? clock})
    : _clock = clock ?? SystemMonotonicClock();

  final SimulatedGpsDatasource _ds;
  final MonotonicClock _clock;

  /// Precisão atribuída às posições do simulador, em metros.
  static const double accuracyM = 1;

  @override
  Future<LocationPermissionState> permission() async =>
      LocationPermissionState.granted;

  @override
  Future<LocationPermissionState> requestPermission() async =>
      LocationPermissionState.granted;

  /// Uma posição por notificação válida do simulador, carimbada com o
  /// instante da chegada; [interval] e [background] não se aplicam.
  /// Notificações malformadas ou sem posição válida são descartadas.
  @override
  Stream<GpsFix> fixes({required Duration interval, bool background = false}) =>
      _ds
          .payloads()
          .map(SimulatedGpsPayload.tryDecode)
          .where((p) => p != null && p.valid)
          .map(
            (p) => GpsFix(
              latE7: p!.latE7,
              lonE7: p.lonE7,
              altMm: p.altMm,
              accuracyM: accuracyM,
              at: _clock.now(),
            ),
          )
          .handleError((Object e) {
            throw LocationFailure(
              e is StateError ? e.message : 'Falha ao ler o simulador',
              cause: e,
            );
          });
}
