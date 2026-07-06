import 'dart:async';

import 'package:tccelta_mobile/src/core/errors/obd_failure.dart';
import 'package:tccelta_mobile/src/data/datasources/elm327_client.dart';
import 'package:tccelta_mobile/src/data/datasources/obd2_datasource.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_adapter_info.dart';
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
  /// Cria o repository sobre o [_dongle] (fonte da conexão ativa) e passa a
  /// observar as fases de conexão para teardown proativo.
  Obd2RepositoryImpl(this._dongle) {
    // Teardown proativo: quando o link cai (failed/disconnected), desmonta o
    // Elm327Client sem esperar a próxima leitura. `reconnecting` NÃO desmonta —
    // a recuperação é transparente e reaproveita a mesma conexão/cliente.
    _phaseSub = _dongle.connectionPhase.listen(
      (phase) {
        if (phase == BleConnectionPhase.disconnected ||
            phase == BleConnectionPhase.failed) {
          _teardown();
        }
      },
      onError: (_) => _teardown(),
    );
  }

  final DongleRepository _dongle;

  StreamSubscription<BleConnectionPhase>? _phaseSub;
  BleConnection? _boundConn;
  Elm327Client? _elm;
  Obd2Datasource? _datasource;
  bool _initialized = false;

  /// Identidade capturada da conexão atual (versão no init, protocolo após a
  /// primeira leitura). Zerada no [_teardown] para recapturar na reconexão.
  String? _version;
  String? _protocol;

  @override
  List<Obd2Pid> get pids => Obd2Pid.values;

  @override
  Obd2AdapterInfo? get adapterInfo => (_version == null && _protocol == null)
      ? null
      : Obd2AdapterInfo(version: _version, protocol: _protocol);

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
    _version = null;
    _protocol = null;
  }

  /// Cancela a escuta de fases e desmonta o cliente ativo. Chamado quando o
  /// provider é descartado (fim do ProviderScope).
  void dispose() {
    unawaited(_phaseSub?.cancel());
    _phaseSub = null;
    _teardown();
  }

  @override
  Future<void> initialize() async {
    final ds = _ensureDatasource();
    if (ds == null) {
      throw const ObdCommandFailure('Sem conexão BLE pronta para inicializar');
    }
    if (_initialized) return;
    _version = await ds.initialize();
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
    // O protocolo só é detectável após a primeira troca com o veículo, então é
    // consultado uma única vez, após a primeira leitura bem-sucedida.
    if (_protocol == null && readings.isNotEmpty) {
      _protocol = await ds.describeProtocol();
    }
    return readings;
  }
}
