import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_reading.dart';
import 'package:tccelta_mobile/src/domain/telemetry/indicator_display.dart';
import 'package:tccelta_mobile/src/router/app_routes.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';
import 'package:tccelta_mobile/src/ui/telemetry/view_model/panel_view_model.dart';
import 'package:tccelta_mobile/src/ui/telemetry/view_model/telemetry_view_model.dart';
import 'package:tccelta_mobile/src/ui/telemetry/widgets/indicator_format_sheet.dart';
import 'package:tccelta_mobile/src/ui/telemetry/widgets/telemetry_status_band.dart';

/// Painel de telemetria OBD-II — a tela `/painel`.
///
/// View "burra": observa o [telemetryViewModelProvider] (leituras ao vivo) e o
/// [panelViewModelProvider] (quais PIDs o usuário escolheu exibir, em que
/// ordem, e com que customização — [IndicatorDisplay]) e renderiza cada
/// indicador no widget certo para o seu formato (número, número linha
/// inteira, gauge, barra ou histórico — ver [_IndicatorCell]). O painel
/// começa vazio — o usuário adiciona indicadores pela tela de sensores
/// (`SensorPickerScreen`, empilhada com `context.push`), que abre o
/// [IndicatorFormatSheet] para escolher o formato; tocar num card já presente
/// reabre esse mesmo sheet para editar, direto (sem tela de detalhe/gráfico
/// no meio). Os cards podem ser reordenados por arrastar
/// ([ReorderableCardGrid]). O `ConnectionGuard` global protege a rota; se o
/// link cair, ele redireciona para "Conexão perdida".
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
                    _EmptyPanel(
                      onAdd: () => context.push(AppRoutes.sensorPicker),
                    )
                  else ...[
                    ReorderableCardGrid<Obd2Pid>(
                      items: indicators,
                      keyOf: ValueKey.new,
                      onReorder: ref
                          .read(panelViewModelProvider.notifier)
                          .reorder,
                      spanOf: (pid) => _spanFor(panel.displayFor(pid)),
                      minHeightOf: (pid) =>
                          _minHeightFor(panel.displayFor(pid)),
                      itemBuilder: (context, pid, index) => _IndicatorCell(
                        pid: pid,
                        reading: byPid[pid],
                        display: panel.displayFor(pid),
                        history: telemetry.history[pid] ?? const [],
                        onTap: () => _openEdit(
                          context,
                          ref,
                          pid,
                          panel.displayFor(pid),
                        ),
                      ),
                    ),
                    const SizedBox(height: AppSpacing.s3),
                    _AddIndicatorButton(
                      onTap: () => context.push(AppRoutes.sensorPicker),
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

/// Quantas colunas do grid o indicador [display] ocupa: linha inteira
/// ("4x4") para número linha-inteira, histórico e gauge grande; meia coluna
/// ("2x2", padrão) para todo o resto.
int _spanFor(IndicatorDisplay display) {
  final isFullRow =
      display.format == IndicatorFormat.numberFull ||
      display.format == IndicatorFormat.history ||
      (display.format == IndicatorFormat.gauge &&
          display.gaugeSize == IndicatorGaugeSize.large);
  return isFullRow ? 2 : 1;
}

/// Altura mínima da célula do grid para [display] — maior para o histórico
/// (espaço do gráfico) e o gauge grande, célula normal para o resto.
double _minHeightFor(IndicatorDisplay display) {
  final isBig =
      display.format == IndicatorFormat.history ||
      (display.format == IndicatorFormat.gauge &&
          display.gaugeSize == IndicatorGaugeSize.large);
  return isBig ? 220 : 86;
}

/// Abre o sheet de formato pré-preenchido com [current] para editar [pid], e
/// aplica o resultado ao [PanelViewModel] (edita ou remove). Sem tela de
/// detalhe/gráfico no meio — vai direto para a escolha/edição de formato.
Future<void> _openEdit(
  BuildContext context,
  WidgetRef ref,
  Obd2Pid pid,
  IndicatorDisplay current,
) async {
  final result = await showIndicatorFormatSheet(
    context,
    pid: pid,
    initial: current,
  );
  if (result == null || !context.mounted) return;
  final notifier = ref.read(panelViewModelProvider.notifier);
  if (result.remove) {
    notifier.removeIndicator(pid);
  } else if (result.display != null) {
    notifier.updateDisplay(pid, result.display!);
  }
}

/// Um card do grid do Painel: escolhe o widget certo conforme
/// `display.format` e sobrepõe a alça de arrastar (puramente visual — o
/// arrasto de fato já funciona em qualquer ponto do card, via
/// [ReorderableCardGrid]) no canto superior direito.
class _IndicatorCell extends StatelessWidget {
  const _IndicatorCell({
    required this.pid,
    required this.reading,
    required this.display,
    required this.history,
    required this.onTap,
  });

  final Obd2Pid pid;
  final Obd2Reading? reading;
  final IndicatorDisplay display;
  final List<double> history;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    // `StackFit.expand` força o card a preencher toda a altura que o grid
    // reservou para ele (igualada entre os cards da mesma linha por
    // `ReorderableCardGrid`) — sem isso, o card ficaria só com a própria
    // altura de conteúdo, deixando vão vazio dentro da célula do grid.
    return Stack(
      fit: StackFit.expand,
      children: [
        _card(),
        const Positioned(
          top: 9,
          right: 9,
          child: IgnorePointer(child: DragHandleDots()),
        ),
      ],
    );
  }

  Widget _card() {
    final value = reading?.value;
    return switch (display.format) {
      IndicatorFormat.number => StatCard.value(
        label: pid.label,
        value: value?.round(),
        unit: pid.unit,
        onTap: onTap,
      ),
      IndicatorFormat.numberFull => StatCard.value(
        label: pid.label,
        value: value?.round(),
        unit: pid.unit,
        fullWidth: true,
        onTap: onTap,
      ),
      IndicatorFormat.bar => StatCard.progress(
        label: pid.label,
        pct: _pct(value),
        value: value?.round(),
        unit: pid.unit,
        colorByZone: true,
        onTap: onTap,
      ),
      IndicatorFormat.gauge => _gaugeCard(value),
      IndicatorFormat.history => GestureDetector(
        behavior: HitTestBehavior.opaque,
        onTap: onTap,
        child: StatGraphCard(
          label: pid.label,
          history: _historyPoints(),
          unit: pid.unit,
        ),
      ),
    };
  }

  double _pct(double? value) {
    if (value == null || display.max <= display.min) return 0;
    return ((value - display.min) / (display.max - display.min) * 100).clamp(
      0.0,
      100.0,
    );
  }

  List<double> _historyPoints() {
    if (history.length <= display.historyPoints) return history;
    return history.sublist(history.length - display.historyPoints);
  }

  Widget _gaugeCard(double? value) {
    final large = display.gaugeSize == IndicatorGaugeSize.large;
    final gauge = Gauge(
      value: value ?? display.min,
      max: display.max,
      label: pid.label,
      unit: pid.unit,
      variant: gaugeVariantFor(display.gaugeStyle),
      warningThreshold: display.lowFraction,
      alertThreshold: display.highFraction,
      invertZones: pid.higherIsBetter,
      size: large ? 172 : 54,
      // No card pequeno, tipo (label) e número já aparecem ao lado do
      // instrumento (StatCard) — o gauge desenha só o arco, sem duplicar.
      showValue: large,
    );
    return StatCard.gauge(
      label: pid.label,
      gauge: gauge,
      value: value?.round(),
      unit: pid.unit,
      centered: large,
      onTap: onTap,
    );
  }
}
