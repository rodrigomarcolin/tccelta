import 'dart:async';

import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/core/errors/obd_failure.dart';
import 'package:tccelta_mobile/src/data/repositories/obd2_repository_impl.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_adapter_state.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_device.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';
import 'package:tccelta_mobile/src/domain/repositories/dongle_repository.dart';

import '../../support/scripted_ble_connection.dart';

/// [DongleRepository] falso que só expõe a conexão ativa (o que o
/// [Obd2RepositoryImpl] consome). Os demais membros não são exercitados.
class _FakeDongleRepository implements DongleRepository {
  _FakeDongleRepository(this._conn, {Stream<BleConnectionPhase>? phase})
      : _phase = phase ?? const Stream.empty();

  final BleConnection? _conn;
  final Stream<BleConnectionPhase> _phase;

  @override
  BleConnection? get connection => _conn;

  @override
  bool get isReady => _conn?.isReady ?? false;

  @override
  Stream<BleAdapterState> get adapterState => const Stream.empty();

  @override
  Stream<BleConnectionPhase> get connectionPhase => _phase;

  @override
  Stream<List<BleDevice>> scan({
    Duration timeout = const Duration(seconds: 15),
  }) =>
      const Stream.empty();

  @override
  Future<void> stopScan() async {}

  @override
  Future<void> connect(String deviceId) async {}

  @override
  Future<void> disconnect() async {}
}

/// Respostas do mock do firmware para os 6 PIDs + init.
const Map<String, String> _mockResponses = {
  'ATZ': 'ELM327 v1.5\r>',
  'ATE0': 'OK\r>',
  'ATDP': 'ISO 15765-4 (CAN 11/500)\r>',
  '0100': '41 00 18 1E 80 00\r>',
  '0104': '41 04 66\r>',
  '0105': '41 05 82\r>',
  '010C': '41 0C 17 70\r>',
  '010D': '41 0D 3C\r>',
  '010E': '41 0E 94\r>',
  '0111': '41 11 33\r>',
};

void main() {
  group('Obd2RepositoryImpl', () {
    test('readAll decodifica os valores de exemplo', () async {
      final conn = ScriptedBleConnection(responses: _mockResponses);
      final repo = Obd2RepositoryImpl(_FakeDongleRepository(conn));

      final readings = await repo.readAll();
      final byPid = {for (final r in readings) r.pid: r.value};

      expect(byPid[Obd2Pid.engineLoad], closeTo(40, 0.5));
      expect(byPid[Obd2Pid.coolantTemp], 90);
      expect(byPid[Obd2Pid.rpm], 1500);
      expect(byPid[Obd2Pid.speed], 60);
      expect(byPid[Obd2Pid.timingAdvance], 10);
      expect(byPid[Obd2Pid.throttle], closeTo(20, 0.5));
    });

    test('readAll captura versão e protocolo do adaptador', () async {
      final conn = ScriptedBleConnection(responses: _mockResponses);
      final repo = Obd2RepositoryImpl(_FakeDongleRepository(conn));

      expect(repo.adapterInfo, isNull);
      await repo.readAll();

      expect(repo.adapterInfo?.version, 'ELM327 v1.5');
      expect(repo.adapterInfo?.protocol, 'ISO 15765-4 (CAN 11/500)');
    });

    test('readAll omite PID que responde NO DATA', () async {
      final conn = ScriptedBleConnection(
        responses: {..._mockResponses, '010D': 'NO DATA\r>'},
      );
      final repo = Obd2RepositoryImpl(_FakeDongleRepository(conn));

      final readings = await repo.readAll();

      expect(readings.length, 5);
      expect(readings.any((r) => r.pid == Obd2Pid.speed), isFalse);
    });

    test('read de um único PID decodifica corretamente', () async {
      final conn = ScriptedBleConnection(responses: _mockResponses);
      final repo = Obd2RepositoryImpl(_FakeDongleRepository(conn));

      final reading = await repo.read(Obd2Pid.rpm);

      expect(reading.pid, Obd2Pid.rpm);
      expect(reading.value, 1500);
    });

    test('sem conexão pronta lança ObdCommandFailure', () async {
      final repo = Obd2RepositoryImpl(_FakeDongleRepository(null));

      await expectLater(repo.readAll(), throwsA(isA<ObdCommandFailure>()));
      await expectLater(
        repo.read(Obd2Pid.rpm),
        throwsA(isA<ObdCommandFailure>()),
      );
    });

    test('discoverSupported mapeia o bitmask p/ o enum (dropa 0x0F)', () async {
      final conn = ScriptedBleConnection(responses: _mockResponses);
      final repo = Obd2RepositoryImpl(_FakeDongleRepository(conn));

      final supported = await repo.discoverSupported();

      expect(supported, {
        Obd2Pid.engineLoad,
        Obd2Pid.coolantTemp,
        Obd2Pid.rpm,
        Obd2Pid.speed,
        Obd2Pid.timingAdvance,
        Obd2Pid.throttle,
      });
      // Fixa o protocolo (a descoberta faz a 1ª troca OBD).
      expect(repo.adapterInfo?.protocol, 'ISO 15765-4 (CAN 11/500)');
    });

    test('discoverSupported varre o próximo range quando sinalizado', () async {
      final conn = ScriptedBleConnection(
        responses: {
          'ATZ': 'ELM327 v1.5\r>',
          'ATE0': 'OK\r>',
          'ATDP': 'ISO 15765-4 (CAN 11/500)\r>',
          '0100': '41 00 00 18 00 01\r>', // rpm+speed, flag de próximo range
          '0120': '41 20 80 00 00 00\r>', // PID 0x21 (desconhecido) → dropado
        },
      );
      final repo = Obd2RepositoryImpl(_FakeDongleRepository(conn));

      final supported = await repo.discoverSupported();

      expect(supported, {Obd2Pid.rpm, Obd2Pid.speed});
      expect(conn.written, containsAll(['0100', '0120']));
    });

    test('readAll após descoberta lê só os PIDs suportados', () async {
      final conn = ScriptedBleConnection(
        responses: {
          ..._mockResponses,
          '0100': '41 00 00 18 00 00\r>', // só rpm+speed, sem próximo range
        },
      );
      final repo = Obd2RepositoryImpl(_FakeDongleRepository(conn));

      await repo.discoverSupported();
      final readings = await repo.readAll();

      expect(
        readings.map((r) => r.pid).toSet(),
        {Obd2Pid.rpm, Obd2Pid.speed},
      );
    });

    test('desmonta o cliente proativamente ao cair a conexão', () async {
      final conn = ScriptedBleConnection(responses: _mockResponses);
      final phaseCtrl = StreamController<BleConnectionPhase>.broadcast();
      addTearDown(phaseCtrl.close);
      final repo = Obd2RepositoryImpl(
        _FakeDongleRepository(conn, phase: phaseCtrl.stream),
      );

      // Uma leitura constrói o Elm327Client, que assina a TX da conexão.
      await repo.readAll();
      expect(conn.hasIncomingListener, isTrue);
      expect(repo.adapterInfo, isNotNull);

      // Uma fase terminal deve desmontar o cliente sem esperar outra leitura.
      phaseCtrl.add(BleConnectionPhase.disconnected);
      await pumpEventQueue();

      expect(conn.hasIncomingListener, isFalse);
      expect(repo.adapterInfo, isNull);
    });
  });
}
