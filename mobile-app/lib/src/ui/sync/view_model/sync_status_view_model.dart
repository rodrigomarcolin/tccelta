import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_status.dart';
import 'package:tccelta_mobile/src/ui/sync/sync_providers.dart';

/// Expõe o [SyncStatus] do coordenador à UI.
class SyncStatusViewModel extends Notifier<SyncStatus> {
  @override
  SyncStatus build() {
    final coordinator = ref.read(syncCoordinatorProvider);
    final sub = coordinator.status.listen((s) => state = s);
    ref.onDispose(() => unawaited(sub.cancel()));
    return coordinator.current;
  }

  /// Ação de depuração: refaz o handshake agora.
  Future<void> forceHandshake() =>
      ref.read(syncCoordinatorProvider).forceHandshake();

  /// Ação de depuração: o servidor de bancada esquece o estado dos dongles.
  void forgetBenchDongles() =>
      ref.read(benchSyncServerProvider).forgetDongles();
}

/// Provider do [SyncStatusViewModel].
final NotifierProvider<SyncStatusViewModel, SyncStatus>
syncStatusViewModelProvider = NotifierProvider<SyncStatusViewModel, SyncStatus>(
  SyncStatusViewModel.new,
);
