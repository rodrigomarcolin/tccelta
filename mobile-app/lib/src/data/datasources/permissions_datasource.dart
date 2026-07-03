import 'dart:io' show Platform;

import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:permission_handler/permission_handler.dart';

/// Envelopa o plugin `permission_handler` com o que é específico do BLE do app.
/// Não guarda estado — o repository é quem faz isso.
///
/// Android 12+: `BLUETOOTH_SCAN` + `BLUETOOTH_CONNECT` (o manifest declara
/// `neverForLocation`, então não pedimos localização). Em Android ≤11 essas
/// permissões são de instalação e o scan exige localização — declarada no
/// manifest com `maxSdkVersion=30`.
class PermissionsDatasource {
  /// Cria o datasource.
  const PermissionsDatasource();

  /// Permissão de Bluetooth já concedida? Lê o `status` atual **sem** abrir o
  /// diálogo do sistema.
  Future<bool> hasBluetoothPermission() async {
    if (kIsWeb) return true;
    if (Platform.isAndroid) {
      final scan = await Permission.bluetoothScan.status;
      final connect = await Permission.bluetoothConnect.status;
      return _granted(scan) && _granted(connect);
    }
    if (Platform.isIOS) {
      return _granted(await Permission.bluetooth.status);
    }
    return true;
  }

  /// Pede acesso ao BLE (abre o diálogo do sistema). `true` se concedido.
  Future<bool> requestBluetoothPermission() async {
    if (kIsWeb) return true;
    if (Platform.isAndroid) {
      final results = await [
        Permission.bluetoothScan,
        Permission.bluetoothConnect,
      ].request();
      return results.values.every(_granted);
    }
    if (Platform.isIOS) {
      return _granted(await Permission.bluetooth.request());
    }
    return true;
  }

  bool _granted(PermissionStatus s) => s.isGranted || s.isLimited;
}
