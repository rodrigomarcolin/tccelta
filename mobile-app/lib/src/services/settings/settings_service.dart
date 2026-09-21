import 'package:shared_preferences/shared_preferences.dart';
import 'package:tccelta_mobile/src/domain/ble/security_mode.dart';

/// Persists the pre-shared key (PSK) used for AES-256-GCM encryption.
///
/// The key is stored as a 64-char hex string (representing 32 bytes).
/// An absent or empty value means that a secure connection cannot be created.
class SettingsService {
  static const String _pskKey = 'psk_hex';
  static const String _securityModeKey = 'security_mode';

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

  /// Clears the stored PSK (future connections will be rejected).
  Future<void> clearPsk() async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.remove(_pskKey);
  }

  /// Returns the selected secure transport mode.
  Future<SecurityMode> getSecurityMode() async {
    final prefs = await SharedPreferences.getInstance();
    return SecurityMode.fromStorage(prefs.getString(_securityModeKey));
  }

  /// Persists the secure transport mode for future connections.
  Future<void> setSecurityMode(SecurityMode mode) async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.setString(_securityModeKey, mode.storageValue);
  }
}
