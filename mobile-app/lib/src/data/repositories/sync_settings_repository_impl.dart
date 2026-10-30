import 'dart:async';

import 'package:tccelta_mobile/src/core/errors/settings_failure.dart';
import 'package:tccelta_mobile/src/data/datasources/sync_settings_datasource.dart';
import 'package:tccelta_mobile/src/domain/repositories/sync_settings_repository.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_settings.dart';

/// Impl do [SyncSettingsRepository]: monta o modelo com os padrões, ajusta às
/// faixas (regravando o que estava fora delas) e publica as mudanças.
class SyncSettingsRepositoryImpl implements SyncSettingsRepository {
  /// Cria o repository e carrega o valor persistido.
  SyncSettingsRepositoryImpl(this._ds) {
    final raw = _read();
    _current = raw.clamped();
    if (_current != raw) {
      unawaited(_write(_current).catchError((Object _) {}));
    }
  }

  final SyncSettingsDatasource _ds;
  final StreamController<SyncSettings> _changes =
      StreamController<SyncSettings>.broadcast();
  late SyncSettings _current;

  @override
  SyncSettings get current => _current;

  SyncSettings _read() {
    const d = SyncSettings();
    final latency = _ds.readInt(SyncSettingsDatasource.latencyKey);
    final buffer = _ds.readInt(SyncSettingsDatasource.bufferKey);
    final gps = _ds.readInt(SyncSettingsDatasource.gpsPeriodKey);
    return SyncSettings(
      latency: latency == null ? d.latency : Duration(seconds: latency),
      bufferCapacity: buffer ?? d.bufferCapacity,
      gpsPeriod: gps == null ? d.gpsPeriod : Duration(seconds: gps),
      gpsEnabled:
          _ds.readBool(SyncSettingsDatasource.gpsEnabledKey) ?? d.gpsEnabled,
    );
  }

  Future<void> _write(SyncSettings s) async {
    await _ds.writeInt(SyncSettingsDatasource.latencyKey, s.latency.inSeconds);
    await _ds.writeInt(SyncSettingsDatasource.bufferKey, s.bufferCapacity);
    await _ds.writeInt(
      SyncSettingsDatasource.gpsPeriodKey,
      s.gpsPeriod.inSeconds,
    );
    await _ds.writeBool(
      SyncSettingsDatasource.gpsEnabledKey,
      value: s.gpsEnabled,
    );
  }

  @override
  Stream<SyncSettings> watch() {
    late StreamController<SyncSettings> controller;
    StreamSubscription<SyncSettings>? sub;
    controller = StreamController<SyncSettings>(
      onListen: () {
        controller.add(_current);
        sub = _changes.stream.listen(controller.add);
      },
      onCancel: () => sub?.cancel(),
    );
    return controller.stream;
  }

  @override
  Future<void> save(SyncSettings settings) async {
    final next = settings.clamped();
    try {
      await _write(next);
    } on Object catch (e) {
      throw SettingsWriteFailure('Falha ao salvar as configurações', cause: e);
    }
    _current = next;
    if (!_changes.isClosed) _changes.add(next);
  }
}
