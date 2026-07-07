import 'package:tccelta_mobile/src/data/datasources/elm327_client.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_supported.dart';

/// Datasource OBD-II: envolve o [Elm327Client] com o que é específico do
/// protocolo OBD-II — a sequência de init do adaptador e a **extração dos data
/// bytes** de uma resposta ELM327. Todo o parsing byte-a-byte vive aqui; o
/// domínio só recebe bytes já validados para aplicar a fórmula.
class Obd2Datasource {
  /// Cria o datasource sobre um [Elm327Client] ligado à conexão ativa.
  Obd2Datasource(this._elm);

  final Elm327Client _elm;

  /// Prepara o adaptador: reset (`ATZ`) + echo off (`ATE0`) e devolve a
  /// **identidade** do adaptador (ex.: `ELM327 v1.5`), extraída da resposta do
  /// `ATZ`, ou `null` se indisponível.
  ///
  /// Best-effort: qualquer falha aqui é ignorada (o parser de [readPidRaw] é
  /// tolerante a echo/espaços mesmo sem init), então um `ATZ` lento não trava a
  /// telemetria.
  Future<String?> initialize() async {
    String? version;
    for (final cmd in const ['ATZ', 'ATE0']) {
      try {
        final raw = await _elm.command(
          cmd,
          timeout: const Duration(seconds: 6),
        );
        if (cmd == 'ATZ') version = _cleanIdentity(raw);
      } on Object {
        // Ignora — init é best-effort.
      }
    }
    return version;
  }

  /// Consulta o protocolo do barramento em uso (`ATDP` — describe protocol),
  /// ex.: `ISO 15765-4 (CAN 11/500)`. Devolve `null` quando o adaptador ainda
  /// não detectou o protocolo (`AUTO`/`SEARCHING`/`?`) ou em qualquer erro.
  ///
  /// Só é confiável **após** a primeira troca com o veículo (no modo AUTO o
  /// protocolo só é fixado quando um comando OBD é respondido).
  Future<String?> describeProtocol() async {
    try {
      final raw = await _elm.command('ATDP');
      // Remove um eventual eco do comando (`ATDP`) caso o echo off não tenha
      // pego, e normaliza espaços.
      final text =
          raw.replaceFirst(RegExp('^ATDP', caseSensitive: false), '').trim();
      final upper = text.toUpperCase();
      if (text.isEmpty ||
          text.contains('?') ||
          upper.contains('SEARCHING') ||
          upper == 'AUTO') {
        return null;
      }
      return text;
    } on Object {
      return null;
    }
  }

  /// Extrai a identidade `ELM327 ...` de uma resposta de `ATZ` (que pode vir
  /// com eco do comando e ruído do reset). Devolve `null` se não encontrar.
  static String? _cleanIdentity(String raw) {
    final match =
        RegExp('ELM327[^\r\n]*', caseSensitive: false).firstMatch(raw);
    return match?.group(0)?.trim();
  }

  /// Lê [pid] e devolve os **data bytes** crus (ex.: `[0x17, 0x70]` para RPM),
  /// ou `null` quando não há dado (`NO DATA`/`?`/malformado).
  ///
  /// Normaliza a resposta (maiúsculas, só dígitos hex), pula um eventual eco do
  /// comando localizando o cabeçalho de resposta `41<pid>` (o modo de resposta
  /// é o serviço + 0x40), e converte o restante em pares hex.
  Future<List<int>?> readPidRaw(Obd2Pid pid) => _readServiceBytes(pid.pid);

  /// Descobre os PIDs suportados do Serviço 0x01 lendo o bitmask do PID 0x00 e,
  /// enquanto a flag de próximo range estiver ligada, dos PIDs-meta seguintes
  /// (`0x20`, `0x40`…, teto em `0xC0`). Devolve os números crus suportados —
  /// o mapeamento para o enum conhecido é do repository.
  Future<Set<int>> readSupportedPids() async {
    final supported = <int>{};
    for (var base = 0x00; base <= 0xC0; base += 0x20) {
      final bytes = await _readServiceBytes(base);
      if (bytes == null || bytes.length < 4) break; // range sem resposta.
      supported.addAll(supportedPidNumbersFromBitmap(base, bytes));
      if (!bitmapHasNextRange(bytes)) break;
    }
    return supported;
  }

  /// Lê um PID cru do Serviço 0x01 (`01<pid>`), valida o cabeçalho de resposta
  /// `41<pid>` e devolve os **data bytes**, ou `null` em `NO DATA`/`?`/
  /// malformado. Todo o parsing byte-a-byte vive aqui.
  Future<List<int>?> _readServiceBytes(int pid) async {
    final command = _hex(Obd2Pid.mode) + _hex(pid);
    final raw = (await _elm.command(command)).toUpperCase();
    if (raw.contains('NO DATA') || raw.contains('NODATA')) return null;

    // Mantém só dígitos hex — remove espaços, echo com CR já removido, etc.
    final compact = raw.replaceAll(RegExp('[^0-9A-F]'), '');

    // Cabeçalho da resposta: modo (serviço|0x40) + PID, ex.: "410C".
    final header = _hex(Obd2Pid.responseMode) + _hex(pid);
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
