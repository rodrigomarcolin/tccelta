import 'dart:async';

import 'package:tccelta_mobile/src/core/errors/dtc_failure.dart';
import 'package:tccelta_mobile/src/core/errors/obd_failure.dart';
import 'package:tccelta_mobile/src/data/datasources/dtc_datasource.dart';
import 'package:tccelta_mobile/src/data/datasources/elm327_client.dart';
import 'package:tccelta_mobile/src/data/datasources/obd2_datasource.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_active_entry.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_code_codec.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_freeze_frame_entry.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_snapshot.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_status.dart';
import 'package:tccelta_mobile/src/domain/obd2/ecu_role.dart';
import 'package:tccelta_mobile/src/domain/obd2/elm_response.dart';
import 'package:tccelta_mobile/src/domain/obd2/monitor_status.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_adapter_info.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_reading.dart';
import 'package:tccelta_mobile/src/domain/repositories/dongle_repository.dart';
import 'package:tccelta_mobile/src/domain/repositories/obd2_repository.dart';

/// *Source of truth* da telemetria e do diagnóstico (DTC) — um único
/// repository porque os dois falam o mesmo protocolo ELM327 sobre a mesma
/// conexão. Obtém a [BleConnection] viva do [DongleRepository] e, sobre ela,
/// constrói o [Elm327Client] + [Obd2Datasource] + [DtcDatasource] (que
/// dependem da conexão de runtime, por isso nascem aqui e não num provider —
/// mesmo motivo do adapter BLE), **compartilhando o mesmo [Elm327Client]**
/// entre os dois datasources: um único cliente serializa os comandos na
/// conexão, então telemetria e diagnóstico nunca correm o risco de capturar a
/// resposta um do outro. Decodifica os PIDs via a fórmula do domínio e mapeia
/// erro cru → [ObdCommandFailure]/[DtcReadFailure].
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

  /// PIDs congelados pelo Modo 02 (freeze frame) — fixo, sem descoberta via
  /// PID 0x00 (confirmado no handoff do firmware): carga do motor, temp. do
  /// líquido, rotação, velocidade, temp. do ar de admissão e acelerador.
  static const List<Obd2Pid> _freezeFramePids = [
    Obd2Pid.engineLoad,
    Obd2Pid.coolantTemp,
    Obd2Pid.rpm,
    Obd2Pid.speed,
    Obd2Pid.intakeAirTemp,
    Obd2Pid.throttle,
  ];

  StreamSubscription<BleConnectionPhase>? _phaseSub;
  BleConnection? _boundConn;
  Elm327Client? _elm;
  Obd2Datasource? _datasource;
  DtcDatasource? _dtcDatasource;
  bool _initialized = false;

  /// Identidade capturada da conexão atual (versão no init, protocolo após a
  /// primeira leitura). Zerada no [_teardown] para recapturar na reconexão.
  String? _version;
  String? _protocol;

  /// PIDs suportados descobertos (subconjunto conhecido). `null` até a primeira
  /// descoberta; cacheado por conexão e zerado no [_teardown].
  Set<Obd2Pid>? _supported;

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
      _dtcDatasource = DtcDatasource(elm);
    }
    return _datasource;
  }

  void _teardown() {
    _elm?.dispose();
    _elm = null;
    _datasource = null;
    _dtcDatasource = null;
    _boundConn = null;
    _initialized = false;
    _version = null;
    _protocol = null;
    _supported = null;
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
  Future<Set<Obd2Pid>> discoverSupported() async {
    final cached = _supported;
    if (cached != null) return cached;

    final ds = _ensureDatasource();
    if (ds == null) {
      throw const ObdCommandFailure('Sem conexão BLE pronta para descoberta');
    }
    if (!_initialized) await initialize();

    final raw = await ds.readSupportedPids();
    final supported = <Obd2Pid>{
      for (final n in raw) ?Obd2Pid.fromByte(n),
    };
    _supported = supported;
    // A descoberta faz uma troca OBD (0100), então o protocolo já é detectável.
    _protocol ??= await ds.describeProtocol();
    return supported;
  }

  @override
  Future<Obd2Reading> read(Obd2Pid pid) async {
    final ds = _ensureDatasource();
    if (ds == null) {
      throw const ObdCommandFailure('Sem conexão BLE pronta');
    }
    try {
      final responses = await ds.readPidResponses(pid);
      if (responses.isEmpty || responses.first.payload.isEmpty) {
        throw ObdCommandFailure('Sem dados para ${pid.command}');
      }
      final response = responses.first;
      return Obd2Reading(
        pid: pid,
        value: pid.decode(response.payload),
        ecuId: response.ecuId,
      );
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

    // Se a descoberta já rodou, lê só os PIDs suportados; senão, todos os
    // curados (fallback quando o painel foi aberto sem sondar antes).
    final supported = _supported;
    final toRead = supported == null ? pids : pids.where(supported.contains);
    return _readEach(ds, toRead);
  }

  @override
  Future<List<Obd2Reading>> readMany(List<Obd2Pid> pids) async {
    final ds = _ensureDatasource();
    if (ds == null) {
      throw const ObdCommandFailure('Sem conexão BLE pronta');
    }
    if (!_initialized) await initialize();
    return _readEach(ds, pids);
  }

  /// Lê cada PID de [toRead] sequencialmente, na ordem, omitindo os sem
  /// resposta — leitura parcial é válida. Consulta o protocolo uma única vez
  /// (não importa qual ciclo — completo ou restrito a alguns PIDs — dispara
  /// a primeira leitura bem-sucedida).
  Future<List<Obd2Reading>> _readEach(
    Obd2Datasource ds,
    Iterable<Obd2Pid> toRead,
  ) async {
    final readings = <Obd2Reading>[];
    for (final pid in toRead) {
      try {
        final responses = await ds.readPidResponses(pid);
        for (final response in responses) {
          if (response.payload.isEmpty) continue;
          readings.add(
            Obd2Reading(
              pid: pid,
              value: pid.decode(response.payload),
              ecuId: response.ecuId,
            ),
          );
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

  @override
  Future<DtcSnapshot> readDtc() async {
    final ds = _ensureDatasource();
    final dtcDs = _dtcDatasource;
    if (ds == null || dtcDs == null) {
      throw const DtcReadFailure('Sem conexão BLE pronta');
    }
    if (!_initialized) await initialize();
    try {
      final confirmed = await dtcDs.readDtcResponses(0x03);
      final pending = await dtcDs.readDtcResponses(0x07);
      final permanent = await dtcDs.readDtcResponses(0x0A);

      final active = [
        ..._dtcEntries(confirmed, DtcStatus.confirmed),
        ..._dtcEntries(pending, DtcStatus.pending),
        ..._dtcEntries(permanent, DtcStatus.permanent),
      ];

      return DtcSnapshot(
        active: await _attachFreezeFrame(dtcDs, active),
        milOn: await _readMilFromEcus(ds),
      );
    } on DtcReadFailure {
      rethrow;
    } on Object catch (e) {
      throw DtcReadFailure('Falha ao ler DTCs', cause: e);
    }
  }

  /// Lê o PID 0x01 (status de monitoramento) nas ECUs de motor (ECM) e câmbio
  /// (TCM) e considera o MIL aceso se qualquer uma reportar o bit ligado.
  /// Best-effort: erro na leitura (timeout, ECU ausente) não derruba o
  /// diagnóstico inteiro — assume MIL apagado, igual às demais leituras
  /// parciais desta classe.
  Future<bool> _readMilFromEcus(Obd2Datasource ds) async {
    try {
      final responses = await ds.readMonitorStatusResponses();
      for (final role in EcuRole.values) {
        final response = _responseForEcu(responses, role.responseId);
        if (response != null && monitorStatusMilOn(response.payload)) {
          return true;
        }
      }
      return false;
    } on Object {
      return false;
    }
  }

  /// Anexa o freeze frame (Modo 02) só no DTC que de fato o originou — o
  /// protocolo real só guarda um freeze frame por ECU, então os demais
  /// [active] ficam com `freezeFrame: []`.
  Future<List<DtcActiveEntry>> _attachFreezeFrame(
    DtcDatasource dtcDs,
    List<DtcActiveEntry> active,
  ) async {
    final originResponses = await dtcDs.readFreezeFrameResponses(0x02);
    if (originResponses.isEmpty) return active;
    final freezeFramesByEcu = <int?, List<DtcFreezeFrameEntry>>{};
    for (final origin in originResponses) {
      final freezeFrame = <DtcFreezeFrameEntry>[];
      for (final pid in _freezeFramePids) {
        try {
          final responses = await dtcDs.readFreezeFrameResponses(pid.pid);
          final response = _responseForEcu(responses, origin.ecuId);
          if (response == null || response.payload.length < 2) continue;
          final data = response.payload.sublist(1);
          final reading = Obd2Reading(
            pid: pid,
            value: pid.decode(data),
            ecuId: response.ecuId,
          );
          freezeFrame.add(
            DtcFreezeFrameEntry(
              label: pid.shortLabel,
              value: '${_formatNumber(reading.value)} ${pid.unit}'.trim(),
            ),
          );
        } on Object {
          // PID individual falho — congelamento parcial é válido.
        }
      }
      freezeFramesByEcu[origin.ecuId] = freezeFrame;
    }

    return [
      for (final entry in active)
        ..._withFreezeFrame(entry, originResponses, freezeFramesByEcu),
    ];
  }

  static List<DtcActiveEntry> _dtcEntries(
    List<ElmResponse> responses,
    DtcStatus status,
  ) => [
    for (final response in responses)
      for (final raw in _codesFrom(response.payload))
        DtcActiveEntry(
          code: dtcCodeFromRaw(raw),
          status: status,
          ecuId: response.ecuId,
        ),
  ];

  static List<int> _codesFrom(List<int> payload) {
    if (payload.isEmpty) return const [];
    final count = payload.first;
    if (payload.length < 1 + count * 2) return const [];
    return [
      for (var i = 0; i < count; i++)
        (payload[1 + i * 2] << 8) | payload[2 + i * 2],
    ];
  }

  static ElmResponse? _responseForEcu(
    List<ElmResponse> responses,
    int? ecuId,
  ) {
    if (responses.isEmpty) return null;
    if (ecuId == null) return responses.first;
    for (final response in responses) {
      if (response.ecuId == ecuId) return response;
    }
    return null;
  }

  static Iterable<DtcActiveEntry> _withFreezeFrame(
    DtcActiveEntry entry,
    List<ElmResponse> origins,
    Map<int?, List<DtcFreezeFrameEntry>> freezeFramesByEcu,
  ) sync* {
    for (final origin in origins) {
      final originPayload = origin.payload;
      if (originPayload.length < 3) continue;
      final originCode = (originPayload[1] << 8) | originPayload[2];
      if (entry.code != dtcCodeFromRaw(originCode) ||
          entry.ecuId != origin.ecuId) {
        continue;
      }
      yield entry.copyWith(
        freezeFrame: freezeFramesByEcu[origin.ecuId] ?? const [],
      );
      return;
    }
    yield entry;
  }

  /// Formata um valor físico pra exibição, com separador de milhar pt-BR
  /// (ex.: `2480.0` → `"2.480"`, `46.3` → `"46,3"`) — sem depender de `intl`.
  static String _formatNumber(double value) {
    final isWhole = value == value.roundToDouble();
    final rounded = isWhole
        ? value.round().toString()
        : value.toStringAsFixed(1);
    final parts = rounded.split('.');
    final digits = parts[0].replaceFirst('-', '');
    final sign = parts[0].startsWith('-') ? '-' : '';
    final buffer = StringBuffer();
    for (var i = 0; i < digits.length; i++) {
      if (i > 0 && (digits.length - i) % 3 == 0) buffer.write('.');
      buffer.write(digits[i]);
    }
    final integerPart = '$sign$buffer';
    return parts.length > 1 ? '$integerPart,${parts[1]}' : integerPart;
  }
}
