import 'package:flutter/material.dart';
import 'package:flutter_hooks/flutter_hooks.dart';
import 'package:hooks_riverpod/hooks_riverpod.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';
import 'package:tccelta_mobile/src/domain/telemetry/indicator_display.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';
import 'package:tccelta_mobile/src/ui/telemetry/view_model/telemetry_view_model.dart';

/// Resultado devolvido por [Navigator.pop] ao fechar o
/// [IndicatorFormatSheet].
///
/// `null` (o pop "cru", sem valor — inclusive o de tocar fora do sheet)
/// significa que o usuário cancelou — nada deve ser adicionado/alterado/
/// removido.
@immutable
class IndicatorFormatResult {
  /// Cria o resultado de confirmar [display] (adicionar/editar).
  const IndicatorFormatResult.confirmed(this.display) : remove = false;

  /// Cria o resultado de remover o indicador do painel.
  const IndicatorFormatResult.removed() : display = null, remove = true;

  /// Customização confirmada — nula quando [remove] é `true`.
  final IndicatorDisplay? display;

  /// Se o usuário pediu para remover o indicador do painel.
  final bool remove;
}

/// Abre o sheet de escolha/customização de formato para [pid], empilhado
/// sobre a tela atual — direto, sem nenhuma tela de detalhe/gráfico no meio.
///
/// [initial] nulo abre o fluxo de adicionar (a partir dos defaults do PID via
/// [IndicatorDisplay.defaultFor]); não-nulo abre o fluxo de editar,
/// pré-preenchido e com a opção de remover do painel.
Future<IndicatorFormatResult?> showIndicatorFormatSheet(
  BuildContext context, {
  required Obd2Pid pid,
  IndicatorDisplay? initial,
}) {
  return showModalBottomSheet<IndicatorFormatResult>(
    context: context,
    isScrollControlled: true,
    backgroundColor: Colors.transparent,
    builder: (context) => IndicatorFormatSheet(pid: pid, initial: initial),
  );
}

/// Passos possíveis do assistente, na ordem em que aparecem.
enum _WizardStep {
  /// Escolha do formato (número, número linha inteira, gauge, barra,
  /// histórico).
  format,

  /// Estilo do gauge (anel/arco 270°/ponteiro) + tamanho no grid.
  gaugeStyle,

  /// Escala: min/máx (gauge/barra), zonas baixo/médio (gauge ponteiro) ou
  /// quantidade de pontos (histórico).
  scale,
}

/// Conteúdo do sheet para escolher e customizar o formato de exibição de um
/// indicador do Painel.
///
/// Substitui qualquer tela intermediária de "detalhe/gráfico" — é aberto
/// diretamente ao tocar em adicionar um sensor novo ou em editar um já
/// presente no painel (card do Painel ou linha da tela de sensores).
class IndicatorFormatSheet extends HookConsumerWidget {
  /// Cria o sheet para configurar [pid].
  const IndicatorFormatSheet({required this.pid, this.initial, super.key});

  /// PID sendo configurado.
  final Obd2Pid pid;

  /// Customização atual, quando o indicador já está no painel.
  final IndicatorDisplay? initial;

  bool get _isEditing => initial != null;

  List<_WizardStep> _stepsFor(IndicatorFormat format) {
    final steps = [_WizardStep.format];
    if (format == IndicatorFormat.gauge) steps.add(_WizardStep.gaugeStyle);
    if (format == IndicatorFormat.gauge ||
        format == IndicatorFormat.bar ||
        format == IndicatorFormat.history) {
      steps.add(_WizardStep.scale);
    }
    return steps;
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final draft = useState(initial ?? IndicatorDisplay.defaultFor(pid));
    final stepIndex = useState(0);
    final steps = _stepsFor(draft.value.format);
    final index = stepIndex.value.clamp(0, steps.length - 1);
    final current = steps[index];
    final isLast = index == steps.length - 1;

    final telemetry = ref.watch(telemetryViewModelProvider);
    final liveValue = _liveValue(telemetry, pid);

    void onBack() {
      if (index == 0) {
        Navigator.of(context).pop();
        return;
      }
      stepIndex.value = index - 1;
    }

    void onPrimary() {
      if (!isLast) {
        stepIndex.value = index + 1;
        return;
      }
      Navigator.of(
        context,
      ).pop(IndicatorFormatResult.confirmed(draft.value));
    }

    return FractionallySizedBox(
      heightFactor: 0.92,
      child: DecoratedBox(
        decoration: const BoxDecoration(
          color: AppColors.surfaceCard,
          borderRadius: BorderRadius.vertical(top: Radius.circular(26)),
        ),
        child: SafeArea(
          top: false,
          child: Padding(
            padding: const EdgeInsets.fromLTRB(
              AppSpacing.s7,
              AppSpacing.s4,
              AppSpacing.s7,
              AppSpacing.s7,
            ),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                const _DragHandle(),
                const SizedBox(height: AppSpacing.s4),
                _Header(step: current, index: index, onBack: onBack),
                const SizedBox(height: AppSpacing.s2),
                Text(pid.label, style: AppTypography.title),
                const SizedBox(height: AppSpacing.s7),
                Expanded(
                  child: ListView(
                    children: [
                      switch (current) {
                        _WizardStep.format => _FormatStep(
                          value: draft.value.format,
                          onChanged: (f) =>
                              draft.value = draft.value.copyWith(format: f),
                        ),
                        _WizardStep.gaugeStyle => _GaugeStyleStep(
                          pid: pid,
                          display: draft.value,
                          liveValue: liveValue,
                          onChanged: (d) => draft.value = d,
                        ),
                        _WizardStep.scale => _ScaleStep(
                          pid: pid,
                          display: draft.value,
                          liveValue: liveValue,
                          history: telemetry.history[pid] ?? const [],
                          onChanged: (d) => draft.value = d,
                        ),
                      },
                    ],
                  ),
                ),
                const SizedBox(height: AppSpacing.s4),
                AppButton(
                  key: const Key('indicator_format_primary'),
                  onPressed: onPrimary,
                  child: Text(
                    !isLast
                        ? 'Continuar'
                        : (_isEditing ? 'Salvar' : 'Adicionar'),
                  ),
                ),
                if (_isEditing) ...[
                  const SizedBox(height: AppSpacing.s3),
                  AppButton(
                    key: const Key('indicator_format_remove'),
                    variant: AppButtonVariant.danger,
                    onPressed: () => Navigator.of(
                      context,
                    ).pop(const IndicatorFormatResult.removed()),
                    child: const Text('Remover do painel'),
                  ),
                ],
              ],
            ),
          ),
        ),
      ),
    );
  }

  double? _liveValue(TelemetryState telemetry, Obd2Pid pid) {
    for (final reading in telemetry.readings) {
      if (reading.pid == pid) return reading.value;
    }
    return null;
  }
}

/// Alça de arrastar visual do sheet (drag-to-dismiss), sem lógica própria —
/// o gesto é do [showModalBottomSheet].
class _DragHandle extends StatelessWidget {
  const _DragHandle();

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Container(
        width: 40,
        height: 4,
        decoration: const BoxDecoration(
          color: AppColors.borderStrong,
          borderRadius: AppRadii.brPill,
        ),
      ),
    );
  }
}

/// Cabeçalho do sheet: um chevron de voltar (só a partir do 2º passo — o 1º
/// só se cancela arrastando/tocando fora do sheet) + o nome do passo atual.
class _Header extends StatelessWidget {
  const _Header({
    required this.step,
    required this.index,
    required this.onBack,
  });

  final _WizardStep step;
  final int index;
  final VoidCallback onBack;

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        if (index > 0) ...[
          IconButton(
            icon: const Icon(Icons.arrow_back_ios_new, size: 16),
            color: AppColors.textSecondary,
            padding: EdgeInsets.zero,
            constraints: const BoxConstraints(),
            onPressed: onBack,
          ),
          const SizedBox(width: AppSpacing.s2),
        ],
        Text(
          switch (step) {
            _WizardStep.format => 'Exibir no painel',
            _WizardStep.gaugeStyle => 'Estilo do gauge',
            _WizardStep.scale => 'Escala',
          },
          style: AppTypography.ui(
            const TextStyle(fontSize: 12, color: AppColors.textTertiary),
          ),
        ),
      ],
    );
  }
}

/// Mapeia o estilo de domínio para o `GaugeVariant` da UI.
GaugeVariant gaugeVariantFor(IndicatorGaugeStyle style) => switch (style) {
  IndicatorGaugeStyle.ring => GaugeVariant.ring,
  IndicatorGaugeStyle.arc270 => GaugeVariant.arc270,
  IndicatorGaugeStyle.needle => GaugeVariant.arc180,
};

/// Valor de preview quando não há leitura ao vivo (desconectado/sem dado
/// ainda neste ciclo): 60% do fundo de escala, um ponto visualmente
/// interessante sem ser nem vazio nem no teto.
double previewValueFor(IndicatorDisplay display, double? liveValue) =>
    liveValue ?? (display.min + (display.max - display.min) * 0.6);

/// Passo 1 — escolha do formato.
class _FormatStep extends StatelessWidget {
  const _FormatStep({required this.value, required this.onChanged});

  final IndicatorFormat value;
  final ValueChanged<IndicatorFormat> onChanged;

  static const List<(IndicatorFormat, String, String)> _options = [
    (IndicatorFormat.number, 'Número', 'meia coluna do grid'),
    (
      IndicatorFormat.numberFull,
      'Número — linha inteira',
      'ocupa as duas colunas',
    ),
    (IndicatorFormat.gauge, 'Gauge', 'anel, arco 270° ou ponteiro'),
    (IndicatorFormat.bar, 'Número + barra', 'progresso horizontal'),
    (
      IndicatorFormat.history,
      'Histórico — gráfico',
      'gráfico de linha, linha inteira',
    ),
  ];

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        for (final (format, title, subtitle) in _options) ...[
          CardButton(
            title: title,
            subtitle: subtitle,
            showValue: false,
            showChevron: false,
            leading: _FormatPreviewIcon(format: format),
            selected: value == format,
            trailing: RadioDot(selected: value == format),
            onTap: () => onChanged(format),
          ),
          const SizedBox(height: AppSpacing.s2),
        ],
      ],
    );
  }
}

/// Miniatura de 44×30 do formato, na lista de escolha — o mesmo padrão visual
/// (número mono, mini gauge, mini barra, mini sparkline) usado no card real
/// do Painel, só que em miniatura.
class _FormatPreviewIcon extends StatelessWidget {
  const _FormatPreviewIcon({required this.format});

  final IndicatorFormat format;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: 44,
      height: 30,
      alignment: Alignment.center,
      decoration: BoxDecoration(
        color: AppColors.bgScreen,
        borderRadius: AppRadii.brSm,
        border: Border.all(color: AppColors.borderHairline),
      ),
      child: switch (format) {
        IndicatorFormat.number => _glyph('12', AppColors.textPrimary),
        IndicatorFormat.numberFull => _glyph('123', AppColors.cyan500),
        IndicatorFormat.gauge => const Gauge(
          value: 0.55,
          max: 1,
          label: '',
          size: 22,
          weight: 5,
          showValue: false,
        ),
        IndicatorFormat.bar => _miniBar(),
        IndicatorFormat.history => const SizedBox(
          width: 30,
          child: Sparkline(
            values: [2, 6, 3, 9, 5, 10, 7],
            height: 18,
            strokeWidth: 1.6,
          ),
        ),
      },
    );
  }

  Widget _glyph(String text, Color color) => Text(
    text,
    style: AppTypography.mono(
      TextStyle(fontSize: 13, fontWeight: FontWeight.w700, color: color),
    ),
  );

  Widget _miniBar() => ClipRRect(
    borderRadius: AppRadii.brPill,
    child: Container(
      width: 28,
      height: 6,
      color: AppColors.track,
      child: Align(
        alignment: Alignment.centerLeft,
        child: FractionallySizedBox(
          widthFactor: 0.6,
          child: Container(color: AppColors.cyan500),
        ),
      ),
    ),
  );
}

/// Passo 2 — estilo do gauge (anel/arco 270°/ponteiro) + tamanho no grid.
class _GaugeStyleStep extends StatelessWidget {
  const _GaugeStyleStep({
    required this.pid,
    required this.display,
    required this.liveValue,
    required this.onChanged,
  });

  final Obd2Pid pid;
  final IndicatorDisplay display;
  final double? liveValue;
  final ValueChanged<IndicatorDisplay> onChanged;

  static const List<(IndicatorGaugeStyle, String, String)> _styles = [
    (IndicatorGaugeStyle.ring, 'Anel', 'círculo completo'),
    (IndicatorGaugeStyle.arc270, 'Arco 270°', 'arco aberto embaixo'),
    (IndicatorGaugeStyle.needle, 'Ponteiro', 'arco 180° · faixas pintadas'),
  ];

  @override
  Widget build(BuildContext context) {
    final previewValue = previewValueFor(display, liveValue);
    return Column(
      children: [
        Center(
          child: Gauge(
            value: previewValue,
            max: display.max,
            label: pid.label,
            unit: pid.unit,
            variant: gaugeVariantFor(display.gaugeStyle),
            warningThreshold: display.lowMax / display.max,
            alertThreshold: display.highMin / display.max,
            size: 150,
          ),
        ),
        const SizedBox(height: AppSpacing.s7),
        for (final (style, title, subtitle) in _styles) ...[
          CardButton(
            title: title,
            subtitle: subtitle,
            showValue: false,
            showChevron: false,
            selected: display.gaugeStyle == style,
            trailing: RadioDot(selected: display.gaugeStyle == style),
            onTap: () => onChanged(display.copyWith(gaugeStyle: style)),
          ),
          const SizedBox(height: AppSpacing.s2),
        ],
        const SizedBox(height: AppSpacing.s4),
        Align(
          alignment: Alignment.centerLeft,
          child: Text('TAMANHO', style: AppTypography.overline),
        ),
        const SizedBox(height: AppSpacing.s2),
        SegmentedToggle<IndicatorGaugeSize>(
          options: IndicatorGaugeSize.values,
          value: display.gaugeSize,
          labelOf: (s) => s == IndicatorGaugeSize.small ? 'Menor' : 'Maior',
          onChanged: (s) => onChanged(display.copyWith(gaugeSize: s)),
        ),
      ],
    );
  }
}

/// Passo 3 — escala: min/máx (gauge/barra), zonas baixo/médio (gauge
/// ponteiro) ou quantidade de pontos (histórico).
class _ScaleStep extends StatelessWidget {
  const _ScaleStep({
    required this.pid,
    required this.display,
    required this.liveValue,
    required this.history,
    required this.onChanged,
  });

  final Obd2Pid pid;
  final IndicatorDisplay display;
  final double? liveValue;
  final List<double> history;
  final ValueChanged<IndicatorDisplay> onChanged;

  double get _step {
    final range = display.max - display.min;
    return range > 20 ? (range / 20).roundToDouble() : 1;
  }

  void _setMin(double v) {
    final lowMax = display.lowMax.clamp(v, display.highMin);
    onChanged(display.copyWith(min: v, lowMax: lowMax));
  }

  void _setMax(double v) {
    final highMin = display.highMin.clamp(display.lowMax, v);
    final lowMax = display.lowMax.clamp(display.min, highMin);
    onChanged(display.copyWith(max: v, highMin: highMin, lowMax: lowMax));
  }

  void _setLowMax(double v) {
    final lowMax = v.clamp(display.min, display.highMin);
    onChanged(display.copyWith(lowMax: lowMax));
  }

  void _setHighMin(double v) {
    final highMin = v.clamp(display.lowMax, display.max);
    onChanged(display.copyWith(highMin: highMin));
  }

  @override
  Widget build(BuildContext context) {
    if (display.format == IndicatorFormat.history) {
      return _historyScale(context);
    }
    return Column(
      children: [
        _preview(),
        const SizedBox(height: AppSpacing.s7),
        NumberStepper(
          label: 'Valor mínimo',
          sublabel: 'piso da escala · ${pid.unit}',
          value: display.min,
          min: -100000,
          max: display.max,
          step: _step,
          onChanged: _setMin,
        ),
        const SizedBox(height: AppSpacing.s3),
        NumberStepper(
          label: 'Valor máximo',
          sublabel: 'topo da escala · ${pid.unit}',
          value: display.max,
          min: display.min,
          max: 100000,
          step: _step,
          onChanged: _setMax,
        ),
        if (display.format == IndicatorFormat.gauge &&
            display.gaugeStyle == IndicatorGaugeStyle.needle) ...[
          const SizedBox(height: AppSpacing.s3),
          NumberStepper(
            label: 'Baixo até',
            value: display.lowMax,
            min: display.min,
            max: display.highMin,
            step: _step,
            onChanged: _setLowMax,
          ),
          const SizedBox(height: AppSpacing.s3),
          NumberStepper(
            label: 'Médio até',
            value: display.highMin,
            min: display.lowMax,
            max: display.max,
            step: _step,
            onChanged: _setHighMin,
          ),
          const SizedBox(height: AppSpacing.s2),
          Text(
            'Alto · acima de ${display.highMin.round()} ${pid.unit}',
            style: AppTypography.body.copyWith(color: AppColors.textTertiary),
          ),
        ],
      ],
    );
  }

  Widget _preview() {
    final previewValue = previewValueFor(display, liveValue);
    if (display.format == IndicatorFormat.bar) {
      final pct = display.max > display.min
          ? ((previewValue - display.min) / (display.max - display.min) * 100)
                .clamp(0.0, 100.0)
          : 0.0;
      return StatCard.progress(
        label: pid.label,
        pct: pct,
        value: previewValue.round(),
        unit: pid.unit,
        colorByZone: true,
      );
    }
    return Center(
      child: Gauge(
        value: previewValue,
        max: display.max,
        label: pid.label,
        unit: pid.unit,
        variant: gaugeVariantFor(display.gaugeStyle),
        warningThreshold: display.lowMax / display.max,
        alertThreshold: display.highMin / display.max,
        size: 150,
      ),
    );
  }

  Widget _historyScale(BuildContext context) {
    final previewValue = previewValueFor(display, liveValue);
    final points = history.length >= display.historyPoints
        ? history.sublist(history.length - display.historyPoints)
        : List<double>.filled(display.historyPoints, previewValue);
    return Column(
      children: [
        StatGraphCard(label: pid.label, history: points, unit: pid.unit),
        const SizedBox(height: AppSpacing.s7),
        NumberStepper(
          label: 'Quantidade de pontos',
          sublabel: 'amostras exibidas no gráfico',
          value: display.historyPoints.toDouble(),
          min: HistoryPointsRange.min.toDouble(),
          max: HistoryPointsRange.max.toDouble(),
          step: HistoryPointsRange.step.toDouble(),
          onChanged: (v) =>
              onChanged(display.copyWith(historyPoints: v.round())),
        ),
      ],
    );
  }
}
