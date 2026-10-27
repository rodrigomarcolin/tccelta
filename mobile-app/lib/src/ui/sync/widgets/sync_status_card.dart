import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_status.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';
import 'package:tccelta_mobile/src/ui/sync/view_model/sync_status_view_model.dart';

/// Texto curto de cada fase da sincronização.
String syncPhaseLabel(SyncPhase phase) => switch (phase) {
  SyncPhase.offline => 'Sem link com o dongle',
  SyncPhase.waitingNotice => 'Aguardando o dongle',
  SyncPhase.handshaking => 'Handshake em andamento',
  SyncPhase.handshakePending => 'Handshake pendente',
  SyncPhase.open => 'Sincronizando',
  SyncPhase.resyncing => 'Ressincronizando',
  SyncPhase.stopped => 'Parada (dongle bloqueado)',
};

/// Tom do selo de cada fase.
StatusTone syncPhaseTone(SyncPhase phase) => switch (phase) {
  SyncPhase.open => StatusTone.live,
  SyncPhase.handshaking || SyncPhase.resyncing => StatusTone.warning,
  SyncPhase.handshakePending || SyncPhase.stopped => StatusTone.alert,
  SyncPhase.offline || SyncPhase.waitingNotice => StatusTone.neutral,
};

/// Resumo numérico `buffer n/B_max · ACK x · HEAD y`.
String syncCountsLabel(SyncStatus s) =>
    'buffer ${s.buffered}/${s.bufferCapacity} · ACK ${s.appAck ?? "-"} · '
    'HEAD ${s.headSeq ?? "-"}';

/// Cartão de estado da sincronização, com a fase e os números.
class SyncStatusCard extends ConsumerWidget {
  /// Cria o cartão; [onTap] abre os detalhes.
  const SyncStatusCard({this.onTap, super.key});

  /// Toque no cartão.
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final status = ref.watch(syncStatusViewModelProvider);
    return CardButton(
      icon: AppIconData.sinal,
      title: 'Sincronização',
      subtitle: '${syncPhaseLabel(status.phase)} · ${syncCountsLabel(status)}',
      subtitleMono: false,
      showValue: false,
      trailing: StatusBadge(
        label: status.serverMode,
        tone: syncPhaseTone(status.phase),
      ),
      onTap: onTap,
    );
  }
}
