import 'package:shared_preferences/shared_preferences.dart';

/// Persists the pre-shared key (PSK) used for AES-256-GCM encryption.
///
/// The key is stored as a 64-char hex string (representing 32 bytes).
/// An absent or empty value means plaintext mode (no encryption).
class SettingsService {
  static const String _pskKey = 'psk_hex';

  /// Returns the stored PSK hex string, or `null` if not set.
  Future<String?> getPsk() async {
    final prefs = await SharedPreferences.getInstance();
    final v = prefs.getString(_pskKey);
    return (v == null || v.isEmpty) ? null : v;
  }

  /// Persists [hex] as the active PSK.
  Future<void> setPsk(String hex) async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.setString(_pskKey, hex.trim());
  }

  /// Clears the stored PSK (reverts to plaintext mode).
  Future<void> clearPsk() async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.remove(_pskKey);
  }
}
