import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_reading.dart';

/// Contrato de leitura de telemetria OBD-II (interface no domain, impl no
/// data).
///
/// É a *source of truth* das leituras para a UI: fala ELM327 sobre a
/// `BleConnection` viva, decodifica os PIDs e lança subclasses de `Failure` em
/// erro. A camada de UI (view_model) nunca toca no transporte de bytes.
abstract interface class Obd2Repository {
  /// PIDs lidos pelo painel, na ordem de exibição.
  List<Obd2Pid> get pids;

  /// Prepara o adaptador ELM327 (reset + echo off) sobre a conexão atual.
  /// Idempotente: só executa a sequência de init uma vez por conexão.
  Future<void> initialize();

  /// Lê um único [pid]. Lança `ObdCommandFailure` se não houver conexão pronta
  /// ou se o dongle não devolver dados.
  Future<Obd2Reading> read(Obd2Pid pid);

  /// Lê todos os [pids] sequencialmente (um comando por vez, respeitando o
  /// back-pressure da fila do firmware). PIDs sem resposta são omitidos —
  /// leituras parciais são válidas.
  Future<List<Obd2Reading>> readAll();
}
