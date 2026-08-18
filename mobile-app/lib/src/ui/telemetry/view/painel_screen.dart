import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';
import 'package:tccelta_mobile/src/router/app_routes.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';
import 'package:tccelta_mobile/src/ui/telemetry/view_model/panel_view_model.dart';
import 'package:tccelta_mobile/src/ui/telemetry/view_model/telemetry_view_model.dart';
import 'package:tccelta_mobile/src/ui/telemetry/widgets/sensor_picker_sheet.dart';
import 'package:tccelta_mobile/src/ui/telemetry/widgets/telemetry_status_band.dart';

/// Painel de telemetria OBD-II — a tela `/painel`.
///
/// View "burra": observa o [telemetryViewModelProvider] (leituras ao vivo) e o
/// [panelViewModelProvider] (quais PIDs o usuário escolheu exibir, e em que
/// ordem) e renderiza cada indicador num [StatCard.value]. O painel começa
/// vazio — o usuário adiciona indicadores pelo sheet de sensores
/// ([showSensorPickerSheet]), empilhado sobre esta tela, e pode reordenar os
/// cards por arrastar ([ReorderableCardGrid]). O `ConnectionGuard` global
/// protege a rota; se o link cair, ele redireciona para "Conexão perdida".
class PainelScreen extends ConsumerWidget {
  /// Cria o painel.
  const PainelScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final telemetry = ref.watch(telemetryViewModelProvider);
    final panel = ref.watch(panelViewModelProvider);
    final byPid = {for (final r in telemetry.readings) r.pid: r};
    final indicators = panel.indicators;

    return Scaffold(
      body: SafeArea(
        bottom: false,
        child: Column(
          children: [
            const TelemetryStatusBand(),
            Expanded(
              child: ListView(
                padding: const EdgeInsets.fromLTRB(
                  AppSpacing.s7,
                  AppSpacing.s7,
                  AppSpacing.s7,
                  AppSpacing.s9,
                ),
                children: [
                  Text('Painel', style: AppTypography.heading),
                  const SizedBox(height: AppSpacing.s2),
                  Text(
                    'Leituras OBD-II em tempo real',
                    style: AppTypography.body.copyWith(
                      color: AppColors.textTertiary,
                    ),
                  ),
                  const SizedBox(height: AppSpacing.s7),
                  if (indicators.isEmpty)
                    _EmptyPanel(onAdd: () => showSensorPickerSheet(context))
                  else ...[
                    ReorderableCardGrid<Obd2Pid>(
                      items: indicators,
                      keyOf: ValueKey.new,
                      onReorder:
                          ref.read(panelViewModelProvider.notifier).reorder,
                      itemBuilder: (context, pid, index) => StatCard.value(
                        label: pid.label,
                        value: byPid[pid]?.value.round(),
                        unit: pid.unit,
                      ),
                    ),
                    const SizedBox(height: AppSpacing.s3),
                    _AddIndicatorButton(
                      onTap: () => showSensorPickerSheet(context),
                    ),
                  ],
                ],
              ),
            ),
          ],
        ),
      ),
      bottomNavigationBar: AppTabBar(
        onChanged: (key) {
          if (key == 'mais') context.go(AppRoutes.more);
        },
      ),
    );
  }
}

/// Estado vazio do painel: nenhum indicador adicionado ainda.
class _EmptyPanel extends StatelessWidget {
  const _EmptyPanel({required this.onAdd});

  final VoidCallback onAdd;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: AppSpacing.s9),
      child: Column(
        children: [
          const IconTile(AppIconData.painel, size: 56),
          const SizedBox(height: AppSpacing.s4),
          Text(
            'Painel vazio',
            style: AppTypography.title,
            textAlign: TextAlign.center,
          ),
          const SizedBox(height: AppSpacing.s2),
          Text(
            'Adicione indicadores para acompanhar as leituras do veículo.',
            style: AppTypography.body.copyWith(color: AppColors.textTertiary),
            textAlign: TextAlign.center,
          ),
          const SizedBox(height: AppSpacing.s7),
          _AddIndicatorButton(onTap: onAdd, fullWidth: false),
        ],
      ),
    );
  }
}

/// Botão "Adicionar indicador" — abre o sheet de sensores.
class _AddIndicatorButton extends StatelessWidget {
  const _AddIndicatorButton({required this.onTap, this.fullWidth = true});

  final VoidCallback onTap;
  final bool fullWidth;

  @override
  Widget build(BuildContext context) {
    return AppButton(
      variant: AppButtonVariant.tonal,
      fullWidth: fullWidth,
      icon: const Icon(Icons.add_rounded, size: 18),
      onPressed: onTap,
      child: const Text('Adicionar indicador'),
    );
  }
}
