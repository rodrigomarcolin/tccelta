import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:tccelta_mobile/src/domain/repositories/camera_permissions_repository.dart';
import 'package:tccelta_mobile/src/ui/settings/settings_providers.dart';

/// Fase do fluxo de pedido de permissão de câmera (scanner de QR da PSK).
enum CameraPermissionFlowState {
  /// Verificando se a permissão já foi concedida (estado inicial, silencioso).
  checking,

  /// Ainda não pedido.
  idle,

  /// Pedindo (diálogo do sistema aberto).
  requesting,

  /// Concedido — pode mostrar a câmera.
  granted,

  /// Negado — mostrar orientação/repetir.
  denied,

  /// Negado permanentemente — o diálogo do sistema não aparece mais, só os
  /// ajustes do app resolvem.
  permanentlyDenied,
}

/// ViewModel da permissão de câmera. Espelha o ViewModel de permissão de BLE:
/// ao iniciar, verifica se a permissão já foi concedida (para que a tela seja
/// pulada nesse caso) e, quando pedido, dispara o diálogo do sistema; a tela
/// observa e navega conforme o estado.
class CameraPermissionsViewModel extends Notifier<CameraPermissionFlowState> {
  CameraPermissionsRepository get _repo =>
      ref.read(cameraPermissionsRepositoryProvider);

  @override
  CameraPermissionFlowState build() {
    unawaited(_check());
    return CameraPermissionFlowState.checking;
  }

  Future<void> _check() async {
    final has = await _repo.hasCameraPermission();
    state = has
        ? CameraPermissionFlowState.granted
        : CameraPermissionFlowState.idle;
  }

  /// Pede acesso à câmera. Retorna `true` se concedido. Se negado, distingue
  /// negação simples de negação permanente (a UI troca "Permitir" por "Abrir
  /// ajustes do app" nesse segundo caso).
  Future<bool> request() async {
    state = CameraPermissionFlowState.requesting;
    final granted = await _repo.requestCameraPermission();
    if (granted) {
      state = CameraPermissionFlowState.granted;
      return true;
    }
    final locked = await _repo.isPermanentlyDenied();
    state = locked
        ? CameraPermissionFlowState.permanentlyDenied
        : CameraPermissionFlowState.denied;
    return false;
  }
}

/// Provider do [CameraPermissionsViewModel].
///
/// `autoDispose`: o estado não deve sobreviver à presença da tela — a cada
/// entrada em `/psk-setup/scan-qr/permissions` a permissão é re-checada do
/// zero (mesmo motivo do `blePermissionsViewModelProvider`).
final NotifierProvider<CameraPermissionsViewModel, CameraPermissionFlowState>
cameraPermissionsViewModelProvider =
    NotifierProvider<CameraPermissionsViewModel, CameraPermissionFlowState>(
      CameraPermissionsViewModel.new,
      isAutoDispose: true,
    );
