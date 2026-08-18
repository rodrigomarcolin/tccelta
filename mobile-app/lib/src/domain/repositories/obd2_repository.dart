import 'package:tccelta_mobile/src/domain/obd2/obd2_adapter_info.dart';
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

  /// Identidade do adaptador conectado (versão + protocolo), preenchida ao
  /// longo das leituras. `null` enquanto nada foi capturado.
  Obd2AdapterInfo? get adapterInfo;

  /// Prepara o adaptador ELM327 (reset + echo off) sobre a conexão atual.
  /// Idempotente: só executa a sequência de init uma vez por conexão.
  Future<void> initialize();

  /// Descobre quais PIDs (dentre os que o painel sabe decodificar) a ECU
  /// suporta, lendo o bitmask do PID 0x00 (e ranges seguintes). Idempotente:
  /// cacheia o resultado por conexão. Devolve o conjunto vazio se a descoberta
  /// falhar.
  Future<Set<Obd2Pid>> discoverSupported();

  /// Lê um único [pid]. Lança `ObdCommandFailure` se não houver conexão pronta
  /// ou se o dongle não devolver dados.
  Future<Obd2Reading> read(Obd2Pid pid);

  /// Lê todos os [pids] sequencialmente (um comando por vez, respeitando o
  /// back-pressure da fila do firmware). PIDs sem resposta são omitidos —
  /// leituras parciais são válidas.
  Future<List<Obd2Reading>> readAll();

  /// Lê só os [pids] informados, sequencialmente — mesma semântica de
  /// [readAll] (um comando por vez, PIDs sem resposta omitidos), mas
  /// restrita a um subconjunto explícito. Usado pelo ciclo rápido do painel,
  /// que consulta apenas os indicadores que o usuário tem exibidos, em vez
  /// de varrer o catálogo inteiro a cada volta.
  Future<List<Obd2Reading>> readMany(List<Obd2Pid> pids);
}
