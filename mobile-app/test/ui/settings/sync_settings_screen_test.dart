import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/domain/location/gps_source.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_settings.dart';
import 'package:tccelta_mobile/src/ui/connection/connection_providers.dart';
import 'package:tccelta_mobile/src/ui/settings/settings_providers.dart';
import 'package:tccelta_mobile/src/ui/settings/view/sync_settings_screen.dart';
import 'package:tccelta_mobile/src/ui/sync/sync_providers.dart';

import '../../support/fake_ble_service.dart';
import '../../support/fake_dongle.dart';
import '../../support/fake_location_repository.dart';
import '../../support/fake_sync_server_repository.dart';
import '../../support/fake_sync_settings_repository.dart';

void main() {
  late FakeSyncSettingsRepository repo;

  Future<void> pumpScreen(
    WidgetTester tester, {
    required bool debug,
    GpsSource saved = GpsSource.real,
  }) async {
    tester.view
      ..physicalSize = const Size(800, 3000)
      ..devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    repo = FakeSyncSettingsRepository(SyncSettings(gpsSource: saved));
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          isDebugBuildProvider.overrideWithValue(debug),
          syncSettingsRepositoryProvider.overrideWithValue(repo),
          locationRepositoryProvider.overrideWithValue(
            FakeLocationRepository(),
          ),
          dongleSyncRepositoryProvider.overrideWithValue(FakeDongle()),
          syncServerRepositoryProvider.overrideWithValue(
            FakeSyncServerRepository(),
          ),
          bleServiceProvider.overrideWithValue(FakeBleService()),
        ],
        child: MaterialApp(
          theme: AppTheme.dark,
          home: const SyncSettingsScreen(),
        ),
      ),
    );
    await tester.pump(const Duration(milliseconds: 10));
  }

  testWidgets('em depuração mostra a fonte do GPS e troca para o simulador', (
    tester,
  ) async {
    await pumpScreen(tester, debug: true);
    expect(find.text('Real'), findsOneWidget);
    expect(find.text('Permitida o tempo todo'), findsOneWidget);

    await tester.tap(find.text('Simulador (BeamNG)'));
    await tester.pump(const Duration(milliseconds: 10));

    expect(repo.current.gpsSource, GpsSource.simulador);
    expect(find.text('Permitida o tempo todo'), findsNothing);
  });

  testWidgets('fora da depuração não mostra a fonte e ignora o simulador', (
    tester,
  ) async {
    await pumpScreen(tester, debug: false, saved: GpsSource.simulador);
    expect(find.text('Simulador (BeamNG)'), findsNothing);
    expect(find.text('Fonte'), findsNothing);
    expect(find.text('Permitida o tempo todo'), findsOneWidget);
  });
}
