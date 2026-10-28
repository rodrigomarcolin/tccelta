import 'dart:async';
import 'dart:convert';

import 'package:shared_preferences/shared_preferences.dart';

/// Armazenamento do estado do servidor de bancada entre execuções do app.
///
/// O estado é um mapa JSON simples; quem o grava e o lê decide o formato.
abstract interface class BenchStateStore {
  /// Estado salvo, ou `null` se não há nada (ou se está ilegível).
  Map<String, Object?>? load();

  /// Grava [state], substituindo o anterior.
  Future<void> save(Map<String, Object?> state);
}

/// [BenchStateStore] sobre [SharedPreferences].
class SharedPreferencesBenchStateStore implements BenchStateStore {
  /// Cria o armazenamento sobre as preferências compartilhadas.
  SharedPreferencesBenchStateStore(this._prefs);

  static const String _key = 'bench_sync_server_state';

  final SharedPreferences _prefs;

  @override
  Map<String, Object?>? load() {
    final raw = _prefs.getString(_key);
    if (raw == null) return null;
    try {
      final decoded = jsonDecode(raw);
      return decoded is Map<String, Object?> ? decoded : null;
    } on FormatException {
      return null;
    }
  }

  @override
  Future<void> save(Map<String, Object?> state) =>
      _prefs.setString(_key, jsonEncode(state));
}
