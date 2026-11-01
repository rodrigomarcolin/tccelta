import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:tccelta_mobile/src/application/gps/gps_reporter.dart';
import 'package:tccelta_mobile/src/ui/connection/connection_providers.dart';
import 'package:tccelta_mobile/src/ui/settings/settings_providers.dart';
import 'package:tccelta_mobile/src/ui/telemetry/telemetry_providers.dart';

/// Envio periódico do GPS ao dongle; nunca abre o diálogo de permissão.
/// Recriado (e o anterior encerrado) quando o repository de localização muda.
final Provider<GpsReporter> gpsReporterProvider = Provider<GpsReporter>((ref) {
  final reporter = GpsReporter(
    dongle: ref.read(dongleRepositoryProvider),
    location: ref.watch(locationRepositoryProvider),
    obd2: ref.read(obd2RepositoryProvider),
    settings: ref.read(syncSettingsRepositoryProvider),
  )..start();
  ref.onDispose(() => unawaited(reporter.dispose()));
  return reporter;
});
