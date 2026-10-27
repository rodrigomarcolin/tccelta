import 'package:tccelta_mobile/src/core/time/monotonic_clock.dart';

/// [MonotonicClock] controlável: devolve [value] ou, se houver, o [source].
class FakeMonotonicClock implements MonotonicClock {
  /// Cria o relógio; [source] (ex.: `() => async.elapsed` num `fakeAsync`)
  /// tem precedência sobre [value].
  FakeMonotonicClock({this.source});

  /// Valor manual.
  Duration value = Duration.zero;

  /// Fonte do tempo, se houver.
  Duration Function()? source;

  @override
  Duration now() => source?.call() ?? value;
}
