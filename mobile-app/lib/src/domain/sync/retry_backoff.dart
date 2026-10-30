import 'dart:math';

import 'package:tccelta_mobile/src/domain/sync/sync_protocol.dart';

/// Espera exponencial entre tentativas (1 s, 2 s, 4 s … teto de 5 min) com
/// *jitter* de ±10 %.
class RetryBackoff {
  /// Cria o backoff; [random] é injetável para testes.
  RetryBackoff({Random? random}) : _random = random ?? Random();

  final Random _random;
  int _attempt = 0;

  /// Próxima espera; avança a progressão.
  Duration next() {
    final base = kRetryMin.inMilliseconds * (1 << min(_attempt, 20));
    if (_attempt < 20) _attempt++;
    final jitter = 0.9 + 0.2 * _random.nextDouble();
    final ms = min((base * jitter).round(), kRetryMax.inMilliseconds);
    return Duration(milliseconds: ms);
  }

  /// Volta à menor espera.
  void reset() => _attempt = 0;
}
