import 'dart:async';
import 'dart:convert' show ascii;

import 'package:tccelta_mobile/src/core/errors/obd_failure.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';

/// Cliente da linha de comando ELM327 sobre uma [BleConnection] já `ready`.
///
/// Esta é a abstração das **interações byte-a-byte**: encapsula o transporte
/// cru (escrever `"<cmd>\r"` na RX, remontar os chunks de `incoming` da TX) e a
/// correlação request↔response, expondo um único método assíncrono
/// [command] que devolve o texto da resposta. É o `Elm327(conn.write)` que os
/// comentários da Fase 2 antecipavam.
///
/// Os comandos são **serializados** (um em voo por vez): a fila do firmware tem
/// profundidade finita e descarta comandos sob back-pressure, então enfileirar
/// evita que uma rajada de comandos se atropele e se perca.
class Elm327Client {
  /// Cria o cliente e passa a escutar a TX da [_conn].
  Elm327Client(this._conn) {
    _sub = _conn.incoming.listen(_onBytes);
  }

  final BleConnection _conn;
  late final StreamSubscription<List<int>> _sub;

  /// Byte do prompt `>` que o ELM327 emite ao fim de cada resposta.
  static const int _promptByte = 0x3E;

  /// Acumula o texto da resposta em andamento até chegar o prompt.
  final StringBuffer _buffer = StringBuffer();

  /// Completer da resposta corrente (nulo quando não há comando em voo).
  Completer<String>? _pending;
  Timer? _timeout;

  /// Serializa os comandos: encadeia cada envio após o anterior. Uma falha não
  /// envenena a fila (os próximos ainda rodam).
  Future<void> _queue = Future<void>.value();

  bool _disposed = false;

  /// Envia [cmd] (sem terminador) e resolve com o texto da resposta, já sem o
  /// prompt final. Em [timeout] sem resposta, completa com [ObdCommandFailure]
  /// (o comando pode ter sido descartado pelo back-pressure).
  Future<String> command(
    String cmd, {
    Duration timeout = const Duration(seconds: 4),
  }) {
    final result = _queue.then((_) => _send(cmd, timeout));
    _queue = result.then((_) {}, onError: (_) {});
    return result;
  }

  Future<String> _send(String cmd, Duration timeout) {
    if (_disposed) {
      return Future<String>.error(
        const ObdCommandFailure('Cliente ELM327 encerrado'),
      );
    }
    final completer = Completer<String>();
    _pending = completer;
    _buffer.clear();
    _timeout = Timer(timeout, () {
      if (completer.isCompleted) return;
      _pending = null;
      completer.completeError(
        ObdCommandFailure(
          'Timeout aguardando resposta de "$cmd" '
          '(comando pode ter sido descartado)',
        ),
      );
    });
    unawaited(
      _conn.write(ascii.encode('$cmd\r')).catchError((Object e) {
        if (completer.isCompleted) return;
        _timeout?.cancel();
        _pending = null;
        completer.completeError(
          ObdCommandFailure('Falha ao escrever comando "$cmd"', cause: e),
        );
      }),
    );
    return completer.future;
  }

  void _onBytes(List<int> chunk) {
    final pending = _pending;
    if (pending == null) return; // resposta órfã (sem request) — ignora.
    _buffer.write(String.fromCharCodes(chunk));
    // O prompt é sempre o último byte da resposta; qualquer chunk que o
    // contenha fecha a resposta (chunks anteriores já foram acumulados).
    if (!chunk.contains(_promptByte)) return;
    _timeout?.cancel();
    _pending = null;
    final text = _buffer.toString();
    _buffer.clear();
    if (!pending.isCompleted) pending.complete(_stripPrompt(text));
  }

  /// Remove o prompt `>` e o CR/LF que o antecede; deixa echo/espaços intactos
  /// (a normalização de conteúdo é do `Obd2Datasource`).
  static String _stripPrompt(String text) {
    final end = text.indexOf('>');
    final body = end >= 0 ? text.substring(0, end) : text;
    return body.replaceAll('\r', '').replaceAll('\n', ' ').trim();
  }

  /// Encerra a escuta da TX e cancela qualquer comando pendente.
  void dispose() {
    _disposed = true;
    _timeout?.cancel();
    unawaited(_sub.cancel());
    final pending = _pending;
    _pending = null;
    if (pending != null && !pending.isCompleted) {
      pending.completeError(
        const ObdCommandFailure('Cliente ELM327 encerrado'),
      );
    }
  }
}
