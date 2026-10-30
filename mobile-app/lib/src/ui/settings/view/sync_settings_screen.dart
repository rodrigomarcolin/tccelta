import 'package:app_settings/app_settings.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/domain/location/gps_fix.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_settings.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_status.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';
import 'package:tccelta_mobile/src/ui/settings/settings_providers.dart';
import 'package:tccelta_mobile/src/ui/sync/view_model/sync_status_view_model.dart';
import 'package:tccelta_mobile/src/ui/sync/widgets/sync_status_card.dart';

/// Resumo `T_lat · buffer · GPS` usado no subtítulo do cartão da aba "Mais".
final Provider<String> syncSettingsSummaryProvider = Provider<String>((ref) {
  final s = ref.watch(syncSettingsViewModelProvider.select((v) => v.settings));
  final gps = s.gpsEnabled ? 'GPS ${s.gpsPeriod.inSeconds} s' : 'GPS desligado';
  return 'T_lat ${s.latency.inSeconds} s · buffer ${s.bufferCapacity} · $gps';
});

/// Tela "Sincronização": envio ao servidor, GPS, estado e depuração.
class SyncSettingsScreen extends ConsumerWidget {
  /// Cria a tela.
  const SyncSettingsScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(syncSettingsViewModelProvider);
    final vm = ref.read(syncSettingsViewModelProvider.notifier);
    final s = state.settings;

    return Scaffold(
      body: SafeArea(
        child: ListView(
          padding: const EdgeInsets.all(AppSpacing.s7),
          children: [
            Row(
              children: [
                IconButton(
                  onPressed: context.pop,
                  icon: const Icon(Icons.arrow_back),
                  tooltip: 'Voltar',
                ),
                Text('Sincronização', style: AppTypography.heading),
              ],
            ),
            const SizedBox(height: AppSpacing.s5),
            const _SectionTitle('Envio ao servidor'),
            NumberStepper(
              label: 'Latência máxima (T_lat)',
              sublabel: 'tempo máximo de um bloco no buffer · s',
              value: s.latency.inSeconds.toDouble(),
              min: SyncSettings.minLatency.inSeconds.toDouble(),
              max: SyncSettings.maxLatency.inSeconds.toDouble(),
              step: 10,
              onChanged: (v) => vm.setLatency(v.round()),
            ),
            NumberStepper(
              label: 'Buffer (B_max)',
              sublabel: 'blocos guardados no celular · mínimo 2 × 32',
              value: s.bufferCapacity.toDouble(),
              min: SyncSettings.minBuffer.toDouble(),
              max: SyncSettings.maxBuffer.toDouble(),
              step: 32,
              onChanged: (v) => vm.setBufferCapacity(v.round()),
            ),
            const Callout(
              text: 'Novas tentativas: 1 s a 5 min (fixo)',
              tone: StatusTone.neutral,
            ),
            const SizedBox(height: AppSpacing.s7),
            const _SectionTitle('Posição (GPS)'),
            SegmentedToggle<bool>(
              options: const [true, false],
              value: s.gpsEnabled,
              labelOf: (on) => on ? 'Ligado' : 'Desligado',
              onChanged: (on) => vm.setGpsEnabled(enabled: on),
            ),
            NumberStepper(
              label: 'Período (T_GPS)',
              sublabel: 'intervalo entre posições enviadas · s',
              value: s.gpsPeriod.inSeconds.toDouble(),
              min: SyncSettings.minGpsPeriod.inSeconds.toDouble(),
              max: SyncSettings.maxGpsPeriod.inSeconds.toDouble(),
              onChanged: (v) => vm.setGpsPeriod(v.round()),
            ),
            _PermissionRow(
              permission: state.gpsPermission,
              onRequest: vm.requestGpsPermission,
            ),
            const SizedBox(height: AppSpacing.s7),
            const _SectionTitle('Estado'),
            const _StatusSummary(),
            if (kDebugMode) ...[
              const SizedBox(height: AppSpacing.s7),
              const _SectionTitle('Depuração'),
              AppButton(
                variant: AppButtonVariant.secondary,
                onPressed: () => ref
                    .read(syncStatusViewModelProvider.notifier)
                    .forceHandshake(),
                child: const Text('Forçar handshake'),
              ),
              const SizedBox(height: AppSpacing.s3),
              AppButton(
                variant: AppButtonVariant.secondary,
                onPressed: () => ref
                    .read(syncStatusViewModelProvider.notifier)
                    .forgetBenchDongles(),
                child: const Text('Esquecer estado no servidor de bancada'),
              ),
            ],
          ],
        ),
      ),
    );
  }
}

class _SectionTitle extends StatelessWidget {
  const _SectionTitle(this.text);

  final String text;

  @override
  Widget build(BuildContext context) => Padding(
    padding: const EdgeInsets.only(bottom: AppSpacing.s3),
    child: Text(
      text,
      style: AppTypography.label.copyWith(color: AppColors.textTertiary),
    ),
  );
}

class _PermissionRow extends StatelessWidget {
  const _PermissionRow({required this.permission, required this.onRequest});

  final LocationPermissionState? permission;
  final VoidCallback onRequest;

  @override
  Widget build(BuildContext context) {
    final p = permission;
    final (label, tone) = switch (p) {
      LocationPermissionState.granted => (
        'Permitida o tempo todo',
        StatusTone.ok,
      ),
      LocationPermissionState.grantedWhileInUse => (
        'Permitida só em primeiro plano',
        StatusTone.warning,
      ),
      LocationPermissionState.denied => ('Negada', StatusTone.alert),
      LocationPermissionState.deniedForever => (
        'Negada para sempre',
        StatusTone.alert,
      ),
      LocationPermissionState.serviceDisabled => (
        'Localização desligada no aparelho',
        StatusTone.alert,
      ),
      null => ('Verificando', StatusTone.neutral),
    };
    final foreverDenied = p == LocationPermissionState.deniedForever;
    final needsAction = p != LocationPermissionState.granted && p != null;
    return Padding(
      padding: const EdgeInsets.only(top: AppSpacing.s3),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          StatusBadge(label: label, tone: tone),
          if (needsAction) ...[
            const SizedBox(height: AppSpacing.s3),
            AppButton(
              onPressed: foreverDenied
                  ? AppSettings.openAppSettings
                  : onRequest,
              child: Text(
                foreverDenied
                    ? 'Abrir ajustes'
                    : p == LocationPermissionState.grantedWhileInUse
                    ? 'Permitir também em segundo plano'
                    : 'Permitir localização',
              ),
            ),
          ],
        ],
      ),
    );
  }
}

class _StatusSummary extends ConsumerWidget {
  const _StatusSummary();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final st = ref.watch(syncStatusViewModelProvider);
    final source = st.latencySource == LatencySource.server
        ? 'servidor'
        : 'local';
    final lines = <String>[
      'Fase: ${syncPhaseLabel(st.phase)}',
      'DevId: ${st.devId ?? "-"}',
      'HEAD ${st.headSeq ?? "-"} · ACK ${st.appAck ?? "-"}',
      'Último lido: ${st.lastRead ?? "-"}',
      'Buffer ${st.buffered}/${st.bufferCapacity} · em POST ${st.inFlight}',
      'POSTs ok ${st.postsOk} · falhos ${st.postsFailed}',
      'T_lat efetivo: ${st.effectiveLatency.inSeconds} s ($source)',
      'Servidor: ${st.serverMode}',
      if (st.nextAttemptAt != null)
        'Próxima tentativa: ${st.nextAttemptAt!.toLocal()}',
      if (st.lastError != null) 'Último erro: ${st.lastError}',
    ];
    return AppCard(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          for (final line in lines)
            Text(
              line,
              style: AppTypography.body.copyWith(
                color: AppColors.textSecondary,
              ),
            ),
        ],
      ),
    );
  }
}
