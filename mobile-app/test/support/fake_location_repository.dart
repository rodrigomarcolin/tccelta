import 'dart:async';

import 'package:tccelta_mobile/src/domain/location/gps_fix.dart';
import 'package:tccelta_mobile/src/domain/repositories/location_repository.dart';

/// Repository de localização controlado pelo teste.
class FakeLocationRepository implements LocationRepository {
  /// Cria o dublê com a permissão inicial.
  FakeLocationRepository({this.state = LocationPermissionState.granted});

  /// Permissão devolvida por [permission] e [requestPermission].
  LocationPermissionState state;

  final StreamController<GpsFix> _fixes = StreamController<GpsFix>.broadcast();

  /// Quantas vezes o fluxo de posições foi assinado.
  int subscriptions = 0;

  /// Último valor de `background` pedido a [fixes].
  bool? lastBackground;

  /// Entrega uma posição aos assinantes.
  void emit(GpsFix fix) => _fixes.add(fix);

  @override
  Future<LocationPermissionState> permission() async => state;

  @override
  Future<LocationPermissionState> requestPermission() async => state;

  @override
  Stream<GpsFix> fixes({required Duration interval, bool background = false}) {
    subscriptions++;
    lastBackground = background;
    return _fixes.stream;
  }
}
