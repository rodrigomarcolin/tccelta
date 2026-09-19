import 'package:tccelta_mobile/src/domain/obd2/dtc_snapshot.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_adapter_info.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_reading.dart';
import 'package:tccelta_mobile/src/domain/repositories/obd2_repository.dart';

/// [Obd2Repository] falso e determinístico (sem BLE), configurável por
/// [readings]/[dtcSnapshot]/[failure] — cobre tanto o lado de telemetria
/// quanto o de diagnóstico (DTC), já que os dois agora vivem na mesma
/// interface. Usado por telas/view models que só precisam de um repository
/// que responda de forma previsível, sem exercitar o datasource ELM327 real.
class FakeObd2Repository implements Obd2Repository {
  FakeObd2Repository({
    this.readings = const [],
    this.dtcSnapshot = const DtcSnapshot(active: [], milOn: false),
    this.failure,
  });

  /// Leituras devolvidas por [readAll]/[readMany]/[read].
  final List<Obd2Reading> readings;

  /// Retrato devolvido por [readDtc].
  final DtcSnapshot dtcSnapshot;

  /// Se definido, é lançado por qualquer método de leitura (telemetria ou
  /// DTC) em vez do valor configurado.
  final Exception? failure;

  /// Nº de chamadas a [readDtc] — para testes que verificam "reler".
  int readDtcCalls = 0;

  @override
  List<Obd2Pid> get pids => readings.map((r) => r.pid).toList();

  @override
  Obd2AdapterInfo? get adapterInfo => null;

  @override
  Future<void> initialize() async {}

  @override
  Future<Set<Obd2Pid>> discoverSupported() async => pids.toSet();

  @override
  Future<Obd2Reading> read(Obd2Pid pid) async {
    final f = failure;
    if (f != null) throw f;
    return readings.firstWhere((r) => r.pid == pid);
  }

  @override
  Future<List<Obd2Reading>> readAll() async {
    final f = failure;
    if (f != null) throw f;
    return readings;
  }

  @override
  Future<List<Obd2Reading>> readMany(List<Obd2Pid> pids) async {
    final f = failure;
    if (f != null) throw f;
    return readings.where((r) => pids.contains(r.pid)).toList();
  }

  @override
  Future<DtcSnapshot> readDtc() async {
    readDtcCalls++;
    final f = failure;
    if (f != null) throw f;
    return dtcSnapshot;
  }
}
