import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:tccelta_mobile/src/data/datasources/sync_settings_datasource.dart';
import 'package:tccelta_mobile/src/data/repositories/sync_settings_repository_impl.dart';
import 'package:tccelta_mobile/src/domain/location/gps_source.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_settings.dart';

Future<(SharedPreferences, SyncSettingsRepositoryImpl)> _make([
  Map<String, Object> initial = const {},
]) async {
  SharedPreferences.setMockInitialValues(initial);
  final prefs = await SharedPreferences.getInstance();
  return (prefs, SyncSettingsRepositoryImpl(SyncSettingsDatasource(prefs)));
}

void main() {
  test('sem nada salvo valem os padrões', () async {
    final (_, repo) = await _make();
    expect(repo.current, const SyncSettings());
  });

  test('valores gravados são relidos', () async {
    final (prefs, repo) = await _make();
    await repo.save(
      const SyncSettings(
        latency: Duration(seconds: 120),
        bufferCapacity: 128,
        gpsPeriod: Duration(seconds: 5),
        gpsEnabled: false,
      ),
    );
    final again = SyncSettingsRepositoryImpl(SyncSettingsDatasource(prefs));
    expect(again.current.latency, const Duration(seconds: 120));
    expect(again.current.bufferCapacity, 128);
    expect(again.current.gpsPeriod, const Duration(seconds: 5));
    expect(again.current.gpsEnabled, isFalse);
  });

  test('valor fora da faixa é ajustado e regravado', () async {
    final (prefs, repo) = await _make({
      SyncSettingsDatasource.latencyKey: 400,
      SyncSettingsDatasource.bufferKey: 32,
      SyncSettingsDatasource.gpsPeriodKey: 0,
    });
    expect(repo.current.latency, const Duration(seconds: 300));
    expect(repo.current.bufferCapacity, 64);
    expect(repo.current.gpsPeriod, const Duration(seconds: 1));
    await Future<void>.delayed(Duration.zero);
    expect(prefs.getInt(SyncSettingsDatasource.latencyKey), 300);
    expect(prefs.getInt(SyncSettingsDatasource.bufferKey), 64);
    expect(prefs.getInt(SyncSettingsDatasource.gpsPeriodKey), 1);
  });

  test('origem do GPS é gravada em gps_source e relida', () async {
    final (prefs, repo) = await _make();
    await repo.save(const SyncSettings(gpsSource: GpsSource.simulador));
    expect(prefs.getString(SyncSettingsDatasource.gpsSourceKey), 'simulador');
    final again = SyncSettingsRepositoryImpl(SyncSettingsDatasource(prefs));
    expect(again.current.gpsSource, GpsSource.simulador);
  });

  test('origem do GPS desconhecida vira real', () async {
    final (_, repo) = await _make({
      SyncSettingsDatasource.gpsSourceKey: 'marte',
    });
    expect(repo.current.gpsSource, GpsSource.real);
  });

  test('save ajusta às faixas', () async {
    final (_, repo) = await _make();
    await repo.save(const SyncSettings(bufferCapacity: 1000));
    expect(repo.current.bufferCapacity, 512);
  });

  test('watch emite o atual e cada save', () async {
    final (_, repo) = await _make();
    final seen = <SyncSettings>[];
    final sub = repo.watch().listen(seen.add);
    await Future<void>.delayed(Duration.zero);
    await repo.save(const SyncSettings(gpsEnabled: false));
    await Future<void>.delayed(Duration.zero);
    await sub.cancel();
    expect(seen.map((s) => s.gpsEnabled), [true, false]);
  });
}
