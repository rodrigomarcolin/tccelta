import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:tccelta_mobile/src/domain/repositories/permissions_repository.dart';
import 'package:tccelta_mobile/src/ui/connection/connection_providers.dart';

/// Fase do fluxo de pedido de permissões.
enum PermissionFlowState {
  /// Verificando se a permissão já foi concedida (estado inicial, silencioso).
  checking,

  /// Ainda não pedido.
  idle,

  /// Pedindo (diálogo do sistema aberto).
  requesting,

  /// Concedido — pode seguir para a busca.
  granted,

  /// Negado — mostrar orientação/repetir.
  denied,
}

/// ViewModel das permissões de BLE. Ao iniciar, verifica se a permissão já foi
/// concedida (para que a tela seja pulada nesse caso) e, quando pedido, dispara
/// o diálogo do sistema; a tela observa e navega conforme o estado.
class PermissionsViewModel extends Notifier<PermissionFlowState> {
  PermissionsRepository get _repo => ref.read(permissionsRepositoryProvider);

  @override
  PermissionFlowState build() {
    unawaited(_check());
    return PermissionFlowState.checking;
  }

  Future<void> _check() async {
    final has = await _repo.hasBluetoothPermission();
    state = has ? PermissionFlowState.granted : PermissionFlowState.idle;
  }

  /// Pede acesso ao BLE. Retorna `true` se concedido.
  Future<bool> request() async {
    state = PermissionFlowState.requesting;
    final ok = await _repo.requestBluetoothPermission();
    state = ok ? PermissionFlowState.granted : PermissionFlowState.denied;
    return ok;
  }
}

/// Provider do [PermissionsViewModel].
///
/// `autoDispose`: o estado NÃO deve sobreviver à presença da tela. Ao sair de
/// `/permissions` (o `pushReplacement` para a busca), o provider é descartado;
/// ao reentrar (ex.: "Esquecer dispositivo"), o `build()` roda de novo e
/// re-checa a permissão — reproduzindo a transição `checking -> granted` que a
/// tela observa para avançar (sem isso, um estado `granted` remanescente
/// deixaria a tela presa no loading).
final NotifierProvider<PermissionsViewModel, PermissionFlowState>
permissionsViewModelProvider =
    NotifierProvider<PermissionsViewModel, PermissionFlowState>(
      PermissionsViewModel.new,
      isAutoDispose: true,
    );
