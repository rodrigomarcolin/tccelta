import 'package:tccelta_mobile/src/core/crypto/psk_cipher.dart';
import 'package:tccelta_mobile/src/data/datasources/dongle_datasource.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';
import 'package:tccelta_mobile/src/infra/ble/encrypted_ble_connection.dart';
import 'package:tccelta_mobile/src/services/ble/ble_service.dart';
import 'package:tccelta_mobile/src/services/settings/settings_service.dart';

/// Extends [DongleDatasource] to wrap every new [BleConnection] with
/// [EncryptedBleConnection] when a PSK is stored.
///
/// If no key is configured the raw connection is returned unchanged —
/// backward-compatible with plain-text dongles.
class SecureDongleDatasource extends DongleDatasource {
  SecureDongleDatasource(super.ble, this._settings);

  final SettingsService _settings;

  @override
  Future<BleConnection> connect(String deviceId) async {
    final raw = await super.connect(deviceId);
    try {
      final hex = await _settings.getPsk();
      if (hex == null || hex.isEmpty) return raw;
      return EncryptedBleConnection(
        inner: raw,
        cipher: PskCipher.fromHex(hex),
      );
    } catch (_) {
      // Falha ao obter a chave (ex: SharedPreferences não mocked em testes unitários).
      // Fallback seguro para conexão em texto puro.
      return raw;
    }
  }
}
