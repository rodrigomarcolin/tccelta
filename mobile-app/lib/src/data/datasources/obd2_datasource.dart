import 'package:tccelta_mobile/src/data/datasources/elm327_client.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';

/// Datasource OBD-II: envolve o [Elm327Client] com o que é específico do
/// protocolo OBD-II — a sequência de init do adaptador e a **extração dos data
/// bytes** de uma resposta ELM327. Todo o parsing byte-a-byte vive aqui; o
/// domínio só recebe bytes já validados para aplicar a fórmula.
class Obd2Datasource {
  /// Cria o datasource sobre um [Elm327Client] ligado à conexão ativa.
  Obd2Datasource(this._elm);

  final Elm327Client _elm;

  /// Prepara o adaptador: reset (`ATZ`) + echo off (`ATE0`).
  ///
  /// Best-effort: qualquer falha aqui é ignorada (o parser de [readPidRaw] é
  /// tolerante a echo/espaços mesmo sem init), então um `ATZ` lento não trava a
  /// telemetria.
  Future<void> initialize() async {
    for (final cmd in const ['ATZ', 'ATE0']) {
      try {
        await _elm.command(
          cmd,
          timeout: const Duration(seconds: 6),
        );
      } on Object {
        // Ignora — init é best-effort.
      }
    }
  }

  /// Lê [pid] e devolve os **data bytes** crus (ex.: `[0x17, 0x70]` para RPM),
  /// ou `null` quando não há dado (`NO DATA`/`?`/malformado).
  ///
  /// Normaliza a resposta (maiúsculas, só dígitos hex), pula um eventual eco do
  /// comando localizando o cabeçalho de resposta `41<pid>` (o modo de resposta
  /// é o serviço + 0x40), e converte o restante em pares hex.
  Future<List<int>?> readPidRaw(Obd2Pid pid) async {
    final raw = (await _elm.command(pid.command)).toUpperCase();
    if (raw.contains('NO DATA') || raw.contains('NODATA')) return null;

    // Mantém só dígitos hex — remove espaços, echo com CR já removido, etc.
    final compact = raw.replaceAll(RegExp('[^0-9A-F]'), '');

    // Cabeçalho da resposta: modo (serviço|0x40) + PID, ex.: "410C".
    final header = _hex(Obd2Pid.responseMode) + _hex(pid.pid);
    final start = compact.indexOf(header);
    if (start < 0) return null; // '?' ou resposta de outro PID.

    final dataHex = compact.substring(start + header.length);
    if (dataHex.isEmpty || dataHex.length.isOdd) return null;

    return [
      for (var i = 0; i < dataHex.length; i += 2)
        int.parse(dataHex.substring(i, i + 2), radix: 16),
    ];
  }

  static String _hex(int byte) =>
      byte.toRadixString(16).toUpperCase().padLeft(2, '0');
}
