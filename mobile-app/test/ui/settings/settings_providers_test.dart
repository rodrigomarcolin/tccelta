import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/data/repositories/location_repository_impl.dart';
import 'package:tccelta_mobile/src/data/repositories/simulated_location_repository_impl.dart';
import 'package:tccelta_mobile/src/domain/location/gps_source.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_settings.dart';
import 'package:tccelta_mobile/src/ui/connection/connection_providers.dart';
import 'package:tccelta_mobile/src/ui/gps/gps_providers.dart';
import 'package:tccelta_mobile/src/ui/settings/settings_providers.dart';
import 'package:tccelta_mobile/src/ui/telemetry/telemetry_providers.dart';

import '../../support/fake_ble_service.dart';
import '../../support/fake_obd2_repository.dart';
import '../../support/fake_sync_settings_repository.dart';

void main() {
  late FakeSyncSettingsRepository repo;

  ProviderContainer make({required bool debug, required GpsSource saved}) {
    repo = FakeSyncSettingsRepository(SyncSettings(gpsSource: saved));
    final container = ProviderContainer(
      overrides: [
        isDebugBuildProvider.overrideWithValue(debug),
        syncSettingsRepositoryProvider.overrideWithValue(repo),
        bleServiceProvider.overrideWithValue(FakeBleService()),
        obd2RepositoryProvider.overrideWithValue(FakeObd2Repository()),
      ],
    );
    addTearDown(container.dispose);
    return container;
  }

  test('em depuração com simulador usa o repository simulado', () {
    final c = make(debug: true, saved: GpsSource.simulador);
    expect(c.read(effectiveGpsSourceProvider), GpsSource.simulador);
    expect(
      c.read(locationRepositoryProvider),
      isA<SimulatedLocationRepositoryImpl>(),
    );
  });

  test('em depuração com real usa o repository do aparelho', () {
    final c = make(debug: true, saved: GpsSource.real);
    expect(c.read(effectiveGpsSourceProvider), GpsSource.real);
    expect(c.read(locationRepositoryProvider), isA<LocationRepositoryImpl>());
  });

  test('fora da depuração ignora o simulador gravado', () {
    final c = make(debug: false, saved: GpsSource.simulador);
    expect(c.read(effectiveGpsSourceProvider), GpsSource.real);
    expect(c.read(locationRepositoryProvider), isA<LocationRepositoryImpl>());
  });

  test('trocar a origem recria o repository e o reporter de GPS', () async {
    final c = make(debug: true, saved: GpsSource.real);
    final reporter =
        (c
              ..listen(locationRepositoryProvider, (_, _) {})
              ..listen(gpsReporterProvider, (_, _) {}))
            .read(gpsReporterProvider);
    expect(c.read(locationRepositoryProvider), isA<LocationRepositoryImpl>());

    await repo.save(repo.current.copyWith(gpsSource: GpsSource.simulador));
    await Future<void>.delayed(Duration.zero);

    expect(
      c.read(locationRepositoryProvider),
      isA<SimulatedLocationRepositoryImpl>(),
    );
    expect(c.read(gpsReporterProvider), isNot(same(reporter)));
  });
}
