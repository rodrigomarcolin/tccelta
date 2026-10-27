/// Relógio monotônico: só mede intervalos, nunca salta com ajustes de hora.
// ignore: one_member_abstracts
abstract interface class MonotonicClock {
  /// Tempo decorrido desde uma origem arbitrária e fixa.
  Duration now();
}

/// [MonotonicClock] sobre um `Stopwatch` iniciado na construção.
class SystemMonotonicClock implements MonotonicClock {
  /// Cria o relógio e já começa a contar.
  SystemMonotonicClock() : _watch = Stopwatch()..start();

  final Stopwatch _watch;

  @override
  Duration now() => _watch.elapsed;
}
