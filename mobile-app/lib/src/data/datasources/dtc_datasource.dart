import 'package:tccelta_mobile/src/data/datasources/elm327_client.dart';
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
    final command = _hex(service);
    final raw = (await _elm.command(command)).toUpperCase();
    if (raw.contains('NO DATA') || raw.contains('NODATA')) return null;

    final compact = raw.replaceAll(RegExp('[^0-9A-F]'), '');
    final header = _hex(service + 0x40);
    final start = compact.indexOf(header);
    if (start < 0) return null;

    var i = start + header.length;
    if (i + 2 > compact.length) return null;
    final count = int.parse(compact.substring(i, i + 2), radix: 16);
    i += 2;

    final codes = <int>[];
    for (var n = 0; n < count; n++) {
      if (i + 4 > compact.length) return null;
      codes.add(int.parse(compact.substring(i, i + 4), radix: 16));
      i += 4;
    }
    return codes;
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
    final command = '02${_hex(pid)}';
    final raw = (await _elm.command(command)).toUpperCase();
    if (raw.contains('NO DATA') || raw.contains('NODATA')) return null;

    final compact = raw.replaceAll(RegExp('[^0-9A-F]'), '');
    final header = '42${_hex(pid)}';
    final start = compact.indexOf(header);
    if (start < 0) return null;

    final dataHex = compact.substring(start + header.length);
    if (dataHex.isEmpty || dataHex.length.isOdd) return null;

    final bytes = [
      for (var i = 0; i < dataHex.length; i += 2)
        int.parse(dataHex.substring(i, i + 2), radix: 16),
    ];
    // Primeiro byte é o frame# (sempre 0) — descartado, não faz parte do
    // valor.
    if (bytes.isEmpty) return null;
    return bytes.sublist(1);
  }

  static String _hex(int byte) =>
      byte.toRadixString(16).toUpperCase().padLeft(2, '0');
}
