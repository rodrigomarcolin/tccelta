import 'package:flutter_riverpod/misc.dart' show Override;
import 'package:tccelta_mobile/src/ui/settings/settings_providers.dart';
import 'package:tccelta_mobile/src/ui/sync/sync_providers.dart';

import 'fake_dongle.dart';
import 'fake_location_repository.dart';
import 'fake_sync_server_repository.dart';
import 'fake_sync_settings_repository.dart';

/// Overrides que isolam sincronização e GPS do armazenamento e do hardware:
/// telas que montam o `ConnectionGuard` ou a aba "Mais" os exigem.
List<Override> syncProviderOverrides() => [
  syncSettingsRepositoryProvider.overrideWithValue(
    FakeSyncSettingsRepository(),
  ),
  locationRepositoryProvider.overrideWithValue(FakeLocationRepository()),
  dongleSyncRepositoryProvider.overrideWithValue(FakeDongle()),
  syncServerRepositoryProvider.overrideWithValue(FakeSyncServerRepository()),
];
