import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_settings.dart';

void main() {
  test('padrões', () {
    const s = SyncSettings();
    expect(s.latency, const Duration(seconds: 60));
    expect(s.bufferCapacity, 64);
    expect(s.gpsPeriod, const Duration(seconds: 1));
    expect(s.gpsEnabled, isTrue);
  });

  test('clamped nas bordas', () {
    const s = SyncSettings();
    Duration lat(int sec) =>
        s.copyWith(latency: Duration(seconds: sec)).clamped().latency;
    expect(lat(5), const Duration(seconds: 10));
    expect(lat(400), const Duration(seconds: 300));
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
