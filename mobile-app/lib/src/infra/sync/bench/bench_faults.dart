// Os valores vêm de --dart-define e o analisador os avalia como o padrão.
// ignore_for_file: avoid_redundant_argument_values, use_named_constants

/// Falhas injetáveis no servidor de bancada, a cada N chamadas (0 = nunca).
class BenchFaults {
  /// Cria o conjunto de falhas.
  const BenchFaults({
    this.delay = Duration.zero,
    this.fail503Every = 0,
    this.fail401Every = 0,
    this.fail429Every = 0,
  });

  /// Lê `SYNC_BENCH_DELAY_MS`, `SYNC_BENCH_FAIL_503_EVERY`,
  /// `SYNC_BENCH_FAIL_401_EVERY` e `SYNC_BENCH_FAIL_429_EVERY`.
  factory BenchFaults.fromEnvironment() => const BenchFaults(
    delay: Duration(milliseconds: int.fromEnvironment('SYNC_BENCH_DELAY_MS')),
    fail503Every: int.fromEnvironment('SYNC_BENCH_FAIL_503_EVERY'),
    fail401Every: int.fromEnvironment('SYNC_BENCH_FAIL_401_EVERY'),
    fail429Every: int.fromEnvironment('SYNC_BENCH_FAIL_429_EVERY'),
  );

  /// Latência somada a cada chamada.
  final Duration delay;

  /// Responde 503 a cada N chamadas.
  final int fail503Every;

  /// Responde 401 a cada N chamadas.
  final int fail401Every;

  /// Responde 429 a cada N chamadas.
  final int fail429Every;
}
