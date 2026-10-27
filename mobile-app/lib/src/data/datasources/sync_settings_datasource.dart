import 'package:shared_preferences/shared_preferences.dart';

/// Envelopa `shared_preferences` para as configurações de sincronização: só
/// lê e escreve valores crus, sem conhecer o modelo. Não guarda estado.
class SyncSettingsDatasource {
  /// Cria o datasource sobre uma instância já resolvida de
  /// [SharedPreferences].
  const SyncSettingsDatasource(this._prefs);

  final SharedPreferences _prefs;

  /// Chave de `T_lat`, em segundos.
  static const String latencyKey = 'sync_t_lat_s';

  /// Chave de `B_max`, em blocos.
  static const String bufferKey = 'sync_b_max';

  /// Chave de `T_GPS`, em segundos.
  static const String gpsPeriodKey = 'gps_period_s';

  /// Chave do GPS ligado.
  static const String gpsEnabledKey = 'gps_enabled';

  /// Lê um inteiro, ou `null` se ausente.
  int? readInt(String key) => _prefs.getInt(key);

  /// Lê um booleano, ou `null` se ausente.
  bool? readBool(String key) => _prefs.getBool(key);

  /// Grava um inteiro.
  Future<void> writeInt(String key, int value) async {
    await _prefs.setInt(key, value);
  }

  /// Grava um booleano.
  Future<void> writeBool(String key, {required bool value}) async {
    await _prefs.setBool(key, value);
  }
}
