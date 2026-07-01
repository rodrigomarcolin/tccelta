import 'dart:io' show Platform;

import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:permission_handler/permission_handler.dart';

/// Fase do fluxo de pedido de permissões.
enum PermissionFlowState {
  /// Ainda não pedido.
  idle,

  /// Pedindo (diálogo do sistema aberto).
  requesting,

  /// Concedido — pode seguir para a busca.
  granted,

  /// Negado — mostrar orientação/repetir.
  denied,
}

/// ViewModel das permissões de BLE. Pede as permissões necessárias por
/// plataforma e expõe o resultado; a tela observa e navega conforme o estado.
class PermissionsViewModel extends Notifier<PermissionFlowState> {
  @override
  PermissionFlowState build() => PermissionFlowState.idle;

  /// Pede acesso ao BLE. Retorna `true` se concedido.
  ///
  /// Android 12+: `BLUETOOTH_SCAN` + `BLUETOOTH_CONNECT` (o manifest declara
  /// `neverForLocation`, então não pedimos localização). Em Android ≤11 essas
  /// permissões são de instalação e o scan exige localização — declarada no
  /// manifest com `maxSdkVersion=30`.
  Future<bool> request() async {
    state = PermissionFlowState.requesting;
    final ok = await _requestPlatform();
    state = ok ? PermissionFlowState.granted : PermissionFlowState.denied;
    return ok;
  }

  Future<bool> _requestPlatform() async {
    if (kIsWeb) return true;
    if (Platform.isAndroid) {
      final results = await [
        Permission.bluetoothScan,
        Permission.bluetoothConnect,
      ].request();
      return results.values.every((s) => s.isGranted || s.isLimited);
    }
    if (Platform.isIOS) {
      final status = await Permission.bluetooth.request();
      return status.isGranted || status.isLimited;
    }
    return true;
  }
}

/// Provider do [PermissionsViewModel].
final NotifierProvider<PermissionsViewModel, PermissionFlowState>
    permissionsViewModelProvider =
    NotifierProvider<PermissionsViewModel, PermissionFlowState>(
  PermissionsViewModel.new,
);
