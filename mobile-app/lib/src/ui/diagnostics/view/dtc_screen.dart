import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_code.dart';
import 'package:tccelta_mobile/src/router/app_tab_navigation.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';
import 'package:tccelta_mobile/src/ui/diagnostics/view_model/dtc_view_model.dart';
import 'package:tccelta_mobile/src/ui/diagnostics/widgets/diagnostics_status_band.dart';
import 'package:tccelta_mobile/src/ui/diagnostics/widgets/dtc_component_chips_row.dart';
import 'package:tccelta_mobile/src/ui/diagnostics/widgets/dtc_detail_sheet.dart';
import 'package:tccelta_mobile/src/ui/diagnostics/widgets/dtc_empty_state.dart';
import 'package:tccelta_mobile/src/ui/diagnostics/widgets/dtc_group_section.dart';
import 'package:tccelta_mobile/src/ui/diagnostics/widgets/dtc_row.dart';
import 'package:tccelta_mobile/src/ui/diagnostics/widgets/dtc_stats_row.dart';
import 'package:tccelta_mobile/src/ui/diagnostics/widgets/dtc_tab_badge.dart';

/// Aba de Diagnóstico (`/dtc`): códigos de falha (DTCs) do veículo.
///
/// View "burra": observa o [dtcViewModelProvider] e renderiza os 2 cards de
/// resumo, o alternador de vista e os chips de componente, e o corpo — lista
/// "Ativos" ou seções "Todos por componente", conforme `state.viewMode`.
/// Tocar um código abre [showDtcDetailSheet] direto com o `DtcCode` em mãos
/// (sem passar pelo estado do view model). Leitura real — ver
/// `Obd2RepositoryImpl.readDtc`.
class DtcScreen extends ConsumerWidget {
  /// Cria a tela de Diagnóstico.
  const DtcScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final dtc = ref.watch(dtcViewModelProvider);
    final notifier = ref.read(dtcViewModelProvider.notifier);

    return Scaffold(
      body: SafeArea(
        bottom: false,
        child: Column(
          children: [
            const DiagnosticsStatusBand(),
            Expanded(
              child: ListView(
                padding: const EdgeInsets.fromLTRB(
                  AppSpacing.s7,
                  AppSpacing.s7,
                  AppSpacing.s7,
                  AppSpacing.s9,
                ),
                children: [
                  Row(
                    children: [
                      Expanded(
                        child: Text(
                          'Diagnóstico',
                          style: AppTypography.heading,
                        ),
                      ),
                      const SizedBox(width: AppSpacing.s3),
                      AppButton(
                        variant: AppButtonVariant.tonal,
                        fullWidth: false,
                        icon: dtc.isLoading
                            ? null
                            : const AppIcon(
                                AppIconData.recarregar,
                                size: 16,
                              ),
                        onPressed: dtc.isLoading ? null : notifier.reread,
                        child: dtc.isLoading
                            ? const SizedBox(
                                width: 16,
                                height: 16,
                                child: CircularProgressIndicator(
                                  strokeWidth: 2,
                                  color: AppColors.cyan500,
                                ),
                              )
                            : const Text('Reler'),
                      ),
                    ],
                  ),
                  const SizedBox(height: AppSpacing.s5),
                  if (dtc.isLoading && dtc.codes.isEmpty)
                    const Padding(
                      padding: EdgeInsets.symmetric(vertical: AppSpacing.s9),
                      child: Center(child: SpinnerRing(size: 48)),
                    )
                  else ...[
                    DtcStatsRow(
                      activeCount: dtc.activeCount,
                      totalCount: dtc.totalCount,
                      milOn: dtc.milOn,
                    ),
                    const SizedBox(height: AppSpacing.s5),
                    SegmentedToggle<DtcViewMode>(
                      options: DtcViewMode.values,
                      value: dtc.viewMode,
                      labelOf: (mode) => switch (mode) {
                        DtcViewMode.active => 'Ativos',
                        DtcViewMode.byComponent => 'Todos por componente',
                      },
                      onChanged: notifier.setViewMode,
                    ),
                    const SizedBox(height: AppSpacing.s5),
                    DtcComponentChipsRow(
                      selected: dtc.componentFilter,
                      onSelect: notifier.setComponentFilter,
                      activeCount: dtc.activeCount,
                      activeCountFor: dtc.activeCountFor,
                      totalCountFor: dtc.totalCountFor,
                    ),
                    const SizedBox(height: AppSpacing.s5),
                    ..._body(context, dtc),
                  ],
                ],
              ),
            ),
          ],
        ),
      ),
      bottomNavigationBar: AppTabBar(
        active: 'dtc',
        tabs: tabsWithDtcBadge(dtc.activeCount),
        onChanged: (key) => goToAppTab(context, key),
      ),
    );
  }

  List<Widget> _body(BuildContext context, DtcState dtc) {
    void openDetail(DtcCode code) => showDtcDetailSheet(context, dtc: code);

    if (dtc.viewMode == DtcViewMode.active) {
      final active = dtc.activeCodes;
      return [
        if (active.isEmpty)
          const DtcEmptyState()
        else
          for (var i = 0; i < active.length; i++) ...[
            if (i > 0) const SizedBox(height: AppSpacing.s2),
            DtcRow(dtc: active[i], onTap: () => openDetail(active[i])),
          ],
        const SizedBox(height: AppSpacing.s3),
        Text(
          'Pendentes ainda não acenderam a luz — precisam de confirmação em '
          'outro ciclo. Apagar códigos é fase futura.',
          style: AppTypography.ui(
            const TextStyle(fontSize: 11, color: AppColors.textTertiary),
          ),
        ),
      ];
    }

    final grouped = dtc.groupedCodes;
    return [
      for (final entry in grouped.entries) ...[
        DtcGroupSection(
          component: entry.key,
          codes: entry.value,
          onOpen: openDetail,
        ),
        const SizedBox(height: AppSpacing.s5),
      ],
      Text(
        'O catálogo depende do veículo. Códigos em cinza são suportados mas '
        'não estão ativos agora.',
        style: AppTypography.ui(
          const TextStyle(fontSize: 11, color: AppColors.textTertiary),
        ),
      ),
    ];
  }
}
