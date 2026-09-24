import 'package:shared_preferences/shared_preferences.dart';

/// Envelopa `shared_preferences` para a persistência bruta do último dongle
/// conectado: só lê/escreve/apaga uma string, sem conhecer o formato JSON nem
/// os modelos de domínio — essa mecânica fica no repository. Não guarda
/// estado.
class LastDongleDatasource {
  /// Cria o datasource sobre uma instância já resolvida de
  /// [SharedPreferences].
  const LastDongleDatasource(this._prefs);

  final SharedPreferences _prefs;

  static const String _key = 'last_dongle_v1';

  /// Lê o JSON bruto salvo, ou `null` se nada foi persistido ainda.
  String? readRaw() => _prefs.getString(_key);

  /// Grava [raw] (um JSON já serializado) como o novo conteúdo salvo.
  Future<void> writeRaw(String raw) async {
    await _prefs.setString(_key, raw);
  }

  /// Apaga o conteúdo salvo, se houver.
  Future<void> clearRaw() async {
    await _prefs.remove(_key);
  }
}
