import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/domain/location/gps_fix.dart';
import 'package:tccelta_mobile/src/domain/location/gps_source.dart';
import 'package:tccelta_mobile/src/ui/settings/settings_providers.dart';

import '../../support/fake_location_repository.dart';
import '../../support/fake_sync_settings_repository.dart';

void main() {
  late FakeSyncSettingsRepository repo;
  late FakeLocationRepository location;
  late ProviderContainer container;

  setUp(() {
    repo = FakeSyncSettingsRepository();
    location = FakeLocationRepository(
      state: LocationPermissionState.grantedWhileInUse,
    );
    container = ProviderContainer(
      overrides: [
        syncSettingsRepositoryProvider.overrideWithValue(repo),
        locationRepositoryProvider.overrideWithValue(location),
      ],
    );
    addTearDown(container.dispose);
  });

  test('grava T_lat, B_max, T_GPS e liga/desliga o GPS', () async {
    final vm = container.read(syncSettingsViewModelProvider.notifier);
    await vm.setLatency(120);
    await vm.setBufferCapacity(128);
    await vm.setGpsPeriod(5);
    await vm.setGpsEnabled(enabled: false);

    final s = repo.current;
    expect(s.latency, const Duration(seconds: 120));
    expect(s.bufferCapacity, 128);
    expect(s.gpsPeriod, const Duration(seconds: 5));
    expect(s.gpsEnabled, isFalse);
    expect(repo.saves, 4);
  });

  test('grava a origem do GPS', () async {
    final vm = container.read(syncSettingsViewModelProvider.notifier);
    await vm.setGpsSource(GpsSource.simulador);
    expect(repo.current.gpsSource, GpsSource.simulador);
    expect(repo.saves, 1);

    await vm.setGpsSource(GpsSource.real);
    expect(repo.current.gpsSource, GpsSource.real);
  });

  test('consulta e pede a permissão de localização', () async {
    final vm = container.read(syncSettingsViewModelProvider.notifier);
    await vm.refreshPermission();
    expect(
      container.read(syncSettingsViewModelProvider).gpsPermission,
      LocationPermissionState.grantedWhileInUse,
    );

    location.state = LocationPermissionState.granted;
    await vm.requestGpsPermission();
    expect(
      container.read(syncSettingsViewModelProvider).gpsPermission,
      LocationPermissionState.granted,
    );
  });
}
