import 'package:tccelta_mobile/src/core/errors/obd_failure.dart';
import 'package:tccelta_mobile/src/data/datasources/elm327_client.dart';
import 'package:tccelta_mobile/src/data/datasources/obd2_datasource.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_reading.dart';
import 'package:tccelta_mobile/src/domain/repositories/dongle_repository.dart';
import 'package:tccelta_mobile/src/domain/repositories/obd2_repository.dart';

/// *Source of truth* da telemetria. Obtém a [BleConnection] viva do
/// [DongleRepository] e, sobre ela, constrói o [Elm327Client] +
/// [Obd2Datasource] (que dependem da conexão de runtime, por isso nascem aqui e
/// não num provider — mesmo motivo do adapter BLE). Decodifica os PIDs via a
/// fórmula do domínio e mapeia erro cru → [ObdCommandFailure].
class Obd2RepositoryImpl implements Obd2Repository {
  /// Cria o repository sobre o [_dongle] (fonte da conexão ativa).
  Obd2RepositoryImpl(this._dongle);

  final DongleRepository _dongle;

  BleConnection? _boundConn;
  Elm327Client? _elm;
  Obd2Datasource? _datasource;
  bool _initialized = false;

  @override
  List<Obd2Pid> get pids => Obd2Pid.values;

  /// Garante um [Obd2Datasource] ligado à conexão ATUAL, reconstruindo-o quando
  /// a conexão muda de identidade (reconexão) e descartando-o quando não há
  /// conexão pronta. Retorna `null` se não há conexão utilizável.
  Obd2Datasource? _ensureDatasource() {
    final conn = _dongle.connection;
    if (conn == null || !_dongle.isReady) {
      _teardown();
      return null;
    }
    if (!identical(conn, _boundConn)) {
      _teardown();
      _boundConn = conn;
      final elm = Elm327Client(conn);
      _elm = elm;
      _datasource = Obd2Datasource(elm);
    }
    return _datasource;
  }

  void _teardown() {
    _elm?.dispose();
    _elm = null;
    _datasource = null;
    _boundConn = null;
    _initialized = false;
  }

  @override
  Future<void> initialize() async {
    final ds = _ensureDatasource();
    if (ds == null) {
      throw const ObdCommandFailure('Sem conexão BLE pronta para inicializar');
    }
    if (_initialized) return;
    await ds.initialize();
    _initialized = true;
  }

  @override
  Future<Obd2Reading> read(Obd2Pid pid) async {
    final ds = _ensureDatasource();
    if (ds == null) {
      throw const ObdCommandFailure('Sem conexão BLE pronta');
    }
    try {
      final data = await ds.readPidRaw(pid);
      if (data == null) {
        throw ObdCommandFailure('Sem dados para ${pid.command}');
      }
      return Obd2Reading(pid: pid, value: pid.decode(data));
    } on ObdCommandFailure {
      rethrow;
    } on Object catch (e) {
      throw ObdCommandFailure('Falha ao ler ${pid.command}', cause: e);
    }
  }

  @override
  Future<List<Obd2Reading>> readAll() async {
    final ds = _ensureDatasource();
    if (ds == null) {
      throw const ObdCommandFailure('Sem conexão BLE pronta');
    }
    if (!_initialized) await initialize();

    final readings = <Obd2Reading>[];
    for (final pid in pids) {
      try {
        final data = await ds.readPidRaw(pid);
        if (data != null) {
          readings.add(Obd2Reading(pid: pid, value: pid.decode(data)));
        }
      } on Object {
        // PID individual falho é omitido — leitura parcial é válida.
      }
    }
    return readings;
  }
}
