import 'dart:async';

import 'package:fake_async/fake_async.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/core/errors/location_failure.dart';
import 'package:tccelta_mobile/src/data/datasources/simulated_gps_datasource.dart';
import 'package:tccelta_mobile/src/data/repositories/simulated_location_repository_impl.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_device.dart';
import 'package:tccelta_mobile/src/domain/location/gps_fix.dart';

import '../../support/fake_ble_service.dart';
import '../../support/fake_monotonic_clock.dart';

// O anúncio pode trazer o nome do PC em vez de `TCCelta-SIM`.
const _sim = BleDevice(id: 'sim', name: 'DESKTOP-PC', rssi: -50);

/// Vetor ouro: seq 1, válido, −23.5603492, −46.7313340, 728.160 m.
const List<int> _golden = [
  0x01, 0x01, 0x01, 0x00, 0xdc, 0xf9, 0xf4, 0xf1, //
  0x44, 0x5d, 0x25, 0xe4, 0x60, 0x1c, 0x0b, 0x00,
];

typedef _Listen = ({
  List<GpsFix> fixes,
  List<Object> errors,
  StreamSubscription<GpsFix> sub,
});

void main() {
  late FakeBleService ble;
  late SimulatedLocationRepositoryImpl repo;

  void make(FakeAsync async, {List<BleDevice> devices = const [_sim]}) {
    ble = FakeBleService(devices: devices);
    repo = SimulatedLocationRepositoryImpl(
      SimulatedGpsDatasource(ble),
      clock: FakeMonotonicClock(source: () => async.elapsed),
    );
  }

  _Listen listen() {
    final fixes = <GpsFix>[];
    final errors = <Object>[];
    // Cancelada por cada teste, via `cancel`.
    // ignore: cancel_subscriptions
    final sub = repo
        .fixes(interval: const Duration(seconds: 1))
        .listen(fixes.add, onError: errors.add);
    return (fixes: fixes, errors: errors, sub: sub);
  }

  void cancel(FakeAsync async, _Listen r) {
    unawaited(r.sub.cancel());
    async.flushMicrotasks();
  }

  test('permissão é sempre concedida', () async {
    final repo = SimulatedLocationRepositoryImpl(
      SimulatedGpsDatasource(FakeBleService()),
    );
    expect(await repo.permission(), LocationPermissionState.granted);
    expect(await repo.requestPermission(), LocationPermissionState.granted);
  });

  test('emite a posição do vetor ouro com precisão 1 m e o instante', () {
    fakeAsync((async) {
      make(async);
      final r = listen();
      async.elapse(const Duration(milliseconds: 100));
      expect(ble.connectCount, 1);
      expect(ble.lastRequireBond, isFalse);

      async.elapse(const Duration(seconds: 3));
      ble.connections.single.emitIncoming(_golden);
      async.flushMicrotasks();

      final fix = r.fixes.single;
      expect(fix.latE7, -235603492);
      expect(fix.lonE7, -467313340);
      expect(fix.altMm, 728160);
      expect(fix.accuracyM, 1);
      expect(fix.at, async.elapsed);
      expect(r.errors, isEmpty);
      cancel(async, r);
    });
  });

  test('descarta posição inválida e payload malformado', () {
    fakeAsync((async) {
      make(async);
      final r = listen();
      async.elapse(const Duration(milliseconds: 100));
      final conn = ble.connections.single
        ..emitIncoming([..._golden]..[1] = 0)
        ..emitIncoming(List<int>.filled(16, 0))
        ..emitIncoming(_golden.sublist(0, 12));
      async.flushMicrotasks();
      expect(r.fixes, isEmpty);
      expect(r.errors, isEmpty);

      conn.emitIncoming(_golden);
      async.flushMicrotasks();
      expect(r.fixes, hasLength(1));
      cancel(async, r);
    });
  });

  test('cancelar a assinatura desconecta', () {
    fakeAsync((async) {
      make(async);
      final r = listen();
      async.elapse(const Duration(milliseconds: 100));
      final conn = ble.connections.single;
      expect(conn.isDisconnected, isFalse);

      cancel(async, r);
      expect(conn.isDisconnected, isTrue);

      async.elapse(const Duration(seconds: 30));
      expect(ble.connectCount, 1);
    });
  });

  test('simulador ausente vira LocationFailure e a busca se repete', () {
    fakeAsync((async) {
      make(async, devices: const []);
      final r = listen();
      async.flushMicrotasks();
      expect(ble.scanCount, 1);
      expect(r.errors.single, isA<LocationFailure>());

      async.elapse(const Duration(milliseconds: 1900));
      expect(ble.scanCount, 1);
      async.elapse(const Duration(milliseconds: 200));
      expect(ble.scanCount, 2);
      // A espera dobra: 4 s até a próxima busca.
      async.elapse(const Duration(milliseconds: 3800));
      expect(ble.scanCount, 2);
      async.elapse(const Duration(milliseconds: 300));
      expect(ble.scanCount, 3);
      cancel(async, r);
    });
  });

  test('erro do scan vira LocationFailure', () {
    fakeAsync((async) {
      ble = FakeBleService(scanError: Exception('bluetooth desligado'));
      repo = SimulatedLocationRepositoryImpl(SimulatedGpsDatasource(ble));
      final r = listen();
      async.flushMicrotasks();
      expect(r.errors.single, isA<LocationFailure>());
      cancel(async, r);
    });
  });

  test('queda do link vira erro e reconecta após a espera', () {
    fakeAsync((async) {
      make(async);
      final r = listen();
      async.elapse(const Duration(milliseconds: 100));
      final first = ble.connections.single
        ..emitPhase(BleConnectionPhase.failed);
      async.flushMicrotasks();
      expect(first.isDisconnected, isTrue);
      expect(r.errors.single, isA<LocationFailure>());

      async.elapse(const Duration(milliseconds: 2100));
      expect(ble.connectCount, 2);
      ble.connections.last.emitIncoming(_golden);
      async.flushMicrotasks();
      expect(r.fixes, hasLength(1));
      cancel(async, r);
    });
  });
}
