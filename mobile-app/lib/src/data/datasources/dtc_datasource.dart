import 'package:tccelta_mobile/src/data/datasources/elm327_client.dart';
import 'package:tccelta_mobile/src/data/datasources/elm_response_parser.dart';
import 'package:tccelta_mobile/src/domain/obd2/elm_response.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';

/// Datasource de diagnóstico: envolve o [Elm327Client] com o que é específico
/// dos Modos 03/07/0A (lista de DTCs) e 02 (freeze frame) — espelha
/// `Obd2Datasource`, mas para serviços sem PID (03/07/0A) e o formato
/// particular do Modo 02. Todo o parsing byte-a-byte vive aqui; o repository
/// só recebe dado cru (nunca "Pxxxx", nunca string formatada).
///
/// Recebe o **mesmo** [Elm327Client] que o `Obd2Datasource` da conexão atual
/// já usa (injetado pelo repository, não criado aqui) — um único cliente
/// serializa telemetria e diagnóstico na mesma conexão, evitando que duas
/// instâncias concorrentes capturem a resposta uma da outra.
class DtcDatasource {
  /// Cria o datasource sobre um [Elm327Client] ligado à conexão ativa.
  DtcDatasource(this._elm);

  final Elm327Client _elm;

  /// Lê a lista de DTCs de um serviço sem PID (0x03 confirmados/0x07
  /// pendentes/0x0A permanentes). Devolve os códigos crus de 16 bits (ex.:
  /// `0x0301` para P0301) — lista **vazia** (não `null`) quando a leitura
  /// funcionou e não há nenhum DTC (`"43 00"`), e `null` em NO DATA/timeout/
  /// resposta malformada (falha de leitura). Essa distinção importa: 0 DTCs
  /// é sucesso, sem resposta é erro.
  Future<List<int>?> readDtcListRaw(int service) async {
    final responses = await readDtcResponses(service);
    return responses.isEmpty ? null : _codesFrom(responses.first.payload);
  }

  /// Reads all ECU DTC responses, retaining the response header when present.
  ///
  /// Lança [StateError] quando a troca não termina num `4X <contagem>...`
  /// válido (NO DATA/rejeitado/malformado) — Modos 03/07/0A sempre respondem
  /// `4X 00` quando não há DTCs, então qualquer outro desfecho é falha de
  /// leitura, não "zero DTCs" (ver [ElmCompletionReason]). O repository já
  /// mapeia qualquer `Object` daqui pra `DtcReadFailure` (catch-all
  /// existente em `readDtc()`).
  Future<List<ElmResponse>> readDtcResponses(
    int service, {
    int? expectedResponses,
  }) async {
    final suffix =
        expectedResponses != null &&
            expectedResponses >= 1 &&
            expectedResponses <= 8
        ? _hex(expectedResponses)
        : '';
    final raw = await _elm.command('${_hex(service)}$suffix');
    final result = ElmResponseParser.parse(
      raw,
      responseService: service + 0x40,
    );
    if (result.completion != ElmCompletionReason.prompt) {
      throw StateError(
        'Modo ${_hex(service)}: resposta incompleta '
        '(${result.completion.name})',
      );
    }
    return result.responses;
  }

  /// Descobre qual DTC (se algum) tem freeze frame de verdade, perguntando o
  /// PID 0x02 do Modo 02 — tratado à parte pelo protocolo (devolve o DTC de
  /// origem, 2 bytes, não um valor de PID comum). `null` quando não há
  /// freeze frame armazenado (NO DATA).
  Future<int?> readFreezeFrameOriginDtc() async {
    final data = await _readFreezeFrameBytes(0x02);
    if (data == null || data.length < 2) return null;
    return (data[0] << 8) | data[1];
  }

  /// Lê um PID do freeze frame (Modo 02, frame sempre 0) — mesmo formato de
  /// `Obd2Datasource.readPidRaw`: devolve os data bytes crus de [pid], já
  /// congelados no instante da falha, sem nenhuma conversão/formatação.
  /// `null` se o PID não estiver congelado, ou NO DATA.
  Future<List<int>?> readFreezeFramePidRaw(Obd2Pid pid) =>
      _readFreezeFrameBytes(pid.pid);

  /// Lê `"02" + hex(pid)`, valida o cabeçalho `"42" + hex(pid)` e descarta o
  /// byte de frame# (sempre `00` — nem o simulador nem o dongle guardam
  /// histórico de frames antigos), devolvendo só os data bytes.
  Future<List<int>?> _readFreezeFrameBytes(int pid) async {
    final responses = await readFreezeFrameResponses(pid);
    if (responses.isEmpty) return null;
    final bytes = responses.first.payload;
    // Primeiro byte é o frame# (sempre 0) — descartado, não faz parte do
    // valor.
    if (bytes.isEmpty) return null;
    return bytes.sublist(1);
  }

  /// Reads all Mode 02 responses for [pid].
  Future<List<ElmResponse>> readFreezeFrameResponses(int pid) async {
    final raw = await _elm.command('02${_hex(pid)}');
    return ElmResponseParser.parse(
      raw,
      responseService: 0x42,
      pid: pid,
    ).responses;
  }

  static List<int>? _codesFrom(List<int> payload) {
    if (payload.isEmpty) return null;
    final count = payload.first;
    if (payload.length < 1 + count * 2) return null;
    return [
      for (var i = 0; i < count; i++)
        (payload[1 + i * 2] << 8) | payload[2 + i * 2],
    ];
  }

  static String _hex(int byte) =>
      byte.toRadixString(16).toUpperCase().padLeft(2, '0');
}
