import 'dart:typed_data';

import 'package:tccelta_mobile/src/core/crypto/psk_cipher.dart';
import 'package:tccelta_mobile/src/core/errors/ble_failure.dart';
import 'package:tccelta_mobile/src/data/datasources/dongle_datasource.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';
import 'package:tccelta_mobile/src/domain/ble/security_mode.dart';
import 'package:tccelta_mobile/src/infra/ble/encrypted_ble_connection.dart';
import 'package:tccelta_mobile/src/services/settings/settings_service.dart';

/// Extends [DongleDatasource] to wrap every new [BleConnection] with
/// [EncryptedBleConnection] when a PSK is stored.
///
/// A missing or invalid key is a connection failure. Plain-text fallback is
/// deliberately not allowed because it would let an unauthenticated command
/// reach the dongle.
class SecureDongleDatasource extends DongleDatasource {
  /// Creates the datasource over the same `BleService` as [DongleDatasource],
  /// plus the [SettingsService] used to look up the stored PSK.
  SecureDongleDatasource(super._ble, this._settings);

  final SettingsService _settings;

  @override
  Future<BleConnection> connect(String deviceId) async {
    final raw = await super.connect(deviceId);
    try {
      final hex = await _settings.getPsk();
      if (hex == null || hex.isEmpty) {
        await raw.disconnect();
        throw const BleConnectionFailure(
          'PSK não configurada; conexão insegura recusada',
        );
      }
      final mode = await _settings.getSecurityMode();
      final psk = PskCipher.fromHex(hex);
      return switch (mode) {
        SecurityMode.staticPsk => EncryptedBleConnection(
            inner: raw,
            cipher: psk,
            mode: mode,
          ),
        SecurityMode.handshake || SecurityMode.handshakeReplay =>
          EncryptedBleConnection(
            inner: raw,
            psk: pskKey(psk),
            mode: mode,
          ),
      };
    } on BleConnectionFailure {
      rethrow;
    } on Object catch (e) {
      await raw.disconnect();
      throw BleConnectionFailure(
        'Falha ao preparar o canal seguro',
        cause: e,
      );
    }
  }

  /// Extracts the validated PSK for the handshake wrapper without exposing
  /// mutable key state from [PskCipher].
  static Uint8List pskKey(PskCipher cipher) => cipher.keyCopy;
}
import 'dart:typed_data';
