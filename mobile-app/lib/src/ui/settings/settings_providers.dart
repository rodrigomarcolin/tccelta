import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:tccelta_mobile/src/core/persistence/shared_preferences_provider.dart';
import 'package:tccelta_mobile/src/data/datasources/location_datasource.dart';
import 'package:tccelta_mobile/src/data/datasources/simulated_gps_datasource.dart';
import 'package:tccelta_mobile/src/data/datasources/sync_settings_datasource.dart';
import 'package:tccelta_mobile/src/data/repositories/location_repository_impl.dart';
import 'package:tccelta_mobile/src/data/repositories/simulated_location_repository_impl.dart';
import 'package:tccelta_mobile/src/data/repositories/sync_settings_repository_impl.dart';
import 'package:tccelta_mobile/src/domain/location/gps_source.dart';
import 'package:tccelta_mobile/src/domain/repositories/location_repository.dart';
import 'package:tccelta_mobile/src/domain/repositories/sync_settings_repository.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_settings.dart';
import 'package:tccelta_mobile/src/ui/connection/connection_providers.dart';
import 'package:tccelta_mobile/src/ui/settings/view_model/sync_settings_view_model.dart';

/// Build de depuração (`kDebugMode`). Libera as opções só de desenvolvimento.
final Provider<bool> isDebugBuildProvider = Provider<bool>((_) => kDebugMode);

/// Datasource das configurações de sincronização, sobre
/// [sharedPreferencesProvider].
final Provider<SyncSettingsDatasource> syncSettingsDatasourceProvider =
    Provider<SyncSettingsDatasource>(
      (ref) => SyncSettingsDatasource(ref.read(sharedPreferencesProvider)),
    );

/// Repository das configurações (`T_lat`, `B_max`, `T_GPS`, GPS ligado).
/// Não é autoDispose: o coordenador e o GPS o observam durante toda a sessão.
final Provider<SyncSettingsRepository> syncSettingsRepositoryProvider =
    Provider<SyncSettingsRepository>(
      (ref) =>
          SyncSettingsRepositoryImpl(ref.read(syncSettingsDatasourceProvider)),
    );

/// Configurações vigentes, atualizadas a cada gravação.
final StreamProvider<SyncSettings> syncSettingsProvider =
    StreamProvider<SyncSettings>(
      (ref) => ref.watch(syncSettingsRepositoryProvider).watch(),
    );

/// Origem das posições em uso: a configurada em build de depuração; sempre
/// [GpsSource.real] nos demais, qualquer que seja o valor gravado.
final Provider<GpsSource> effectiveGpsSourceProvider = Provider<GpsSource>((
  ref,
) {
  if (!ref.watch(isDebugBuildProvider)) return GpsSource.real;
  final saved = ref.watch(
    syncSettingsProvider.select((s) => s.value?.gpsSource),
  );
  return saved ?? ref.read(syncSettingsRepositoryProvider).current.gpsSource;
});

/// Repository de localização conforme [effectiveGpsSourceProvider]: o GPS do
/// aparelho (plugin `geolocator`) ou o simulador via BLE. Recriado quando a
/// origem muda.
final Provider<LocationRepository> locationRepositoryProvider =
    Provider<LocationRepository>(
      (ref) => switch (ref.watch(effectiveGpsSourceProvider)) {
        GpsSource.real => LocationRepositoryImpl(const LocationDatasource()),
        GpsSource.simulador => SimulatedLocationRepositoryImpl(
          SimulatedGpsDatasource(ref.read(bleServiceProvider)),
        ),
      },
    );

/// ViewModel da tela de configurações de sincronização.
final NotifierProvider<SyncSettingsViewModel, SyncSettingsState>
syncSettingsViewModelProvider =
    NotifierProvider<SyncSettingsViewModel, SyncSettingsState>(
      SyncSettingsViewModel.new,
    );
