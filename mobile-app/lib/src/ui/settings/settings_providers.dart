import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:tccelta_mobile/src/core/persistence/shared_preferences_provider.dart';
import 'package:tccelta_mobile/src/data/datasources/location_datasource.dart';
import 'package:tccelta_mobile/src/data/datasources/sync_settings_datasource.dart';
import 'package:tccelta_mobile/src/data/repositories/location_repository_impl.dart';
import 'package:tccelta_mobile/src/data/repositories/sync_settings_repository_impl.dart';
import 'package:tccelta_mobile/src/domain/repositories/location_repository.dart';
import 'package:tccelta_mobile/src/domain/repositories/sync_settings_repository.dart';
import 'package:tccelta_mobile/src/ui/settings/view_model/sync_settings_view_model.dart';

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

/// Repository de localização, sobre o plugin `geolocator`.
final Provider<LocationRepository> locationRepositoryProvider =
    Provider<LocationRepository>(
      (ref) => LocationRepositoryImpl(const LocationDatasource()),
    );

/// ViewModel da tela de configurações de sincronização.
final NotifierProvider<SyncSettingsViewModel, SyncSettingsState>
syncSettingsViewModelProvider =
    NotifierProvider<SyncSettingsViewModel, SyncSettingsState>(
      SyncSettingsViewModel.new,
    );
