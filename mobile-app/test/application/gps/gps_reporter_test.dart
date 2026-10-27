import 'dart:async';

import 'package:fake_async/fake_async.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/application/gps/gps_reporter.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_adapter_state.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_bond_state.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_device.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_link_error.dart';
import 'package:tccelta_mobile/src/domain/ble/dongle_pairing_hint.dart';
import 'package:tccelta_mobile/src/domain/location/gps_fix.dart';
import 'package:tccelta_mobile/src/domain/repositories/dongle_repository.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_settings.dart';

import '../../support/fake_location_repository.dart';
import '../../support/fake_monotonic_clock.dart';
import '../../support/fake_obd2_repository.dart';
import '../../support/fake_sync_settings_repository.dart';

class _FakeDongle implements DongleRepository {
  bool ready = true;
  final StreamController<BleConnectionPhase> phases =
      StreamController<BleConnectionPhase>.broadcast();

  @override
  bool get isReady => ready;

  @override
  Stream<BleConnectionPhase> get connectionPhase => phases.stream;

  @override
  BleConnection? get connection => null;

  @override
  Stream<BleAdapterState> get adapterState => const Stream.empty();

  @override
  Stream<List<BleDevice>> scan({Duration? timeout}) => const Stream.empty();

  @override
  Future<void> stopScan() async {}

  @override
  Future<void> connect(String deviceId) async {}

  @override
  Future<void> disconnect() async {}

  @override
  bool get canManageBonds => false;

  @override
  Future<BleBondState> bondState(String deviceId) async => BleBondState.unknown;

  @override
  Future<void> removeBond(String deviceId) async {}

  @override
  Stream<DonglePairingHint?> watchPairingHint(
    String deviceId, {
    Duration? timeout,
  }) => const Stream.empty();

  @override
  BleLinkError? get lastLinkError => null;
}

GpsFix _fix(Duration at, {double accuracy = 5}) => GpsFix.fromDegrees(
  lat: -23.5,
  lon: -46.6,
  altM: 760,
  accuracyM: accuracy,
  at: at,
);

void main() {
  test('GpsFix converte graus e metros para as unidades do dongle', () {
    final fix = _fix(Duration.zero);
    expect(fix.latE7, -235000000);
    expect(fix.lonE7, -466000000);
    expect(fix.altMm, 760000);
    expect(fix.toAtCommand(), 'AT#GPS -235000000 -466000000 760000');
  });

  test('envia a posição recente e precisa a cada período', () {
    fakeAsync((async) {
      final clock = FakeMonotonicClock(source: () => async.elapsed);
      final dongle = _FakeDongle();
      final location = FakeLocationRepository();
      final obd2 = FakeObd2Repository();
      final reporter = GpsReporter(
        dongle: dongle,
        location: location,
        obd2: obd2,
        settings: FakeSyncSettingsRepository(),
        clock: clock,
      )..start();
      async.flushMicrotasks();

      location.emit(_fix(clock.now()));
      async.elapse(const Duration(seconds: 1));
      expect(obd2.reportedGps, hasLength(1));
      expect(reporter.current.phase, GpsReportPhase.sending);
      expect(location.lastBackground, isTrue);

      // Ainda vale com 2 períodos de idade; depois disso é velha demais.
      async.elapse(const Duration(seconds: 3));
      expect(obd2.reportedGps, hasLength(2));
      location.emit(_fix(clock.now(), accuracy: 500));
      async.elapse(const Duration(seconds: 1));
      expect(obd2.reportedGps, hasLength(2));

      unawaited(reporter.dispose());
      async.flushMicrotasks();
    });
  });

  test('sem permissão fica parado e não assina o fluxo', () {
    fakeAsync((async) {
      final location = FakeLocationRepository(
        state: LocationPermissionState.denied,
      );
      final reporter = GpsReporter(
        dongle: _FakeDongle(),
        location: location,
        obd2: FakeObd2Repository(),
        settings: FakeSyncSettingsRepository(),
      )..start();
      async.flushMicrotasks();

      expect(reporter.current.phase, GpsReportPhase.noPermission);
      expect(location.subscriptions, 0);
      unawaited(reporter.dispose());
      async.flushMicrotasks();
    });
  });

  test('GPS desligado ou conexão fora de ready ficam parados', () {
    fakeAsync((async) {
      final dongle = _FakeDongle()..ready = false;
      final settings = FakeSyncSettingsRepository();
      final reporter = GpsReporter(
        dongle: dongle,
        location: FakeLocationRepository(),
        obd2: FakeObd2Repository(),
        settings: settings,
      )..start();
      async.flushMicrotasks();
      expect(reporter.current.phase, GpsReportPhase.stopped);

      dongle.ready = true;
      dongle.phases.add(BleConnectionPhase.ready);
      async.flushMicrotasks();
      expect(reporter.current.phase, GpsReportPhase.awaitingFix);

      unawaited(settings.save(const SyncSettings(gpsEnabled: false)));
      async.flushMicrotasks();
      expect(reporter.current.phase, GpsReportPhase.stopped);

      unawaited(reporter.dispose());
      async.flushMicrotasks();
    });
  });
}
