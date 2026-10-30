import 'dart:math';

import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/domain/sync/retry_backoff.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_settings.dart';

/// `Random` fixo em 0,5: o jitter de ±10 % fica neutro.
class _Half implements Random {
  @override
  bool nextBool() => true;
  @override
  double nextDouble() => 0.5;
  @override
  int nextInt(int max) => max ~/ 2;
}

void main() {
  test('1, 2, 4 … 300 s e reset', () {
    final b = RetryBackoff(random: _Half());
    expect(
      [for (var i = 0; i < 12; i++) b.next().inSeconds],
      [
        1, 2, 4, 8, 16, 32, 64, 128, 256, 300, 300, 300, //
      ],
    );
    b.reset();
    expect(b.next(), const Duration(seconds: 1));
  });

  test('jitter fica em ±10 %', () {
    final b = RetryBackoff(random: Random(7));
    for (var i = 0; i < 4; i++) {
      final base = 1000 * (1 << i);
      final ms = b.next().inMilliseconds;
      expect(ms, inInclusiveRange(base * 0.9, base * 1.1));
    }
  });

  test('SyncSettings.clamped nas bordas', () {
    const s = SyncSettings();
    expect(s.latency, const Duration(seconds: 60));
    expect(s.bufferCapacity, 64);
    expect(s.gpsPeriod, const Duration(seconds: 1));
    expect(s.gpsEnabled, isTrue);
    expect(
      s.copyWith(latency: const Duration(seconds: 5)).clamped().latency,
      const Duration(seconds: 10),
    );
    expect(
      s.copyWith(latency: const Duration(seconds: 400)).clamped().latency,
      const Duration(seconds: 300),
    );
    expect(s.copyWith(bufferCapacity: 32).clamped().bufferCapacity, 64);
    expect(s.copyWith(bufferCapacity: 1000).clamped().bufferCapacity, 512);
    expect(
      s.copyWith(gpsPeriod: Duration.zero).clamped().gpsPeriod,
      const Duration(seconds: 1),
    );
    expect(
      s.copyWith(gpsPeriod: const Duration(seconds: 90)).clamped().gpsPeriod,
      const Duration(seconds: 60),
    );
  });
}
