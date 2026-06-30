import 'package:flutter/widgets.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/app_card.dart';

/// Tratamento interno de um [StatCard]. Selecionado pelo construtor nomeado,
/// nunca exposto à API pública.
enum _Variant { value, progress, info, gauge }

/// O pequeno tile de dashboard para uma única leitura.
///
/// Cada tratamento tem seu construtor nomeado, expondo só os campos que usa —
/// não há como passar `pct` num [StatCard.value] nem omitir o gauge num
/// [StatCard.gauge]. Todos os números são mono + tabular.
class StatCard extends StatelessWidget {
  const StatCard._(
    this._variant, {
    required this.label,
    this.value,
    this.unit,
    this.pct = 0,
    this.colorByZone = false,
    this.gauge,
    super.key,
  });

  /// Número grande rotulado (uma leitura).
  const StatCard.value({
    required String label,
    Object? value,
    String? unit,
    Key? key,
  }) : this._(
          _Variant.value,
          label: label,
          value: value,
          unit: unit,
          key: key,
        );

  /// Igual a [StatCard.value] + barra de progresso (leituras percentuais).
  /// [colorByZone] recolore a barra ciano→âmbar→vermelho conforme [pct].
  const StatCard.progress({
    required String label,
    required double pct,
    Object? value,
    String? unit,
    bool colorByZone = false,
    Key? key,
  }) : this._(
          _Variant.progress,
          label: label,
          value: value,
          unit: unit,
          pct: pct,
          colorByZone: colorByZone,
          key: key,
        );

  /// Linha "label … valor mono" para fatos estáticos (protocolo, nº de PIDs).
  const StatCard.info({
    required String label,
    Object? value,
    Key? key,
  }) : this._(
          _Variant.info,
          label: label,
          value: value,
          key: key,
        );

  /// Gauge à esquerda + valor/unidade e label à direita. O instrumento é
  /// passado em [gauge] — qualquer `GaugeVariant` (ou widget) serve.
  const StatCard.gauge({
    required String label,
    required Widget gauge,
    Object? value,
    String? unit,
    Key? key,
  }) : this._(
          _Variant.gauge,
          label: label,
          value: value,
          unit: unit,
          gauge: gauge,
          key: key,
        );

  /// Rótulo (overline) da leitura — ou o label à esquerda no `info`.
  final String label;

  /// Tratamento interno selecionado pelo construtor.
  final _Variant _variant;

  /// Valor da leitura (já formatado) — ou o valor à direita no `info`.
  final Object? value;

  /// Sufixo de unidade (ex.: "%", "°").
  final String? unit;

  /// Percentual de preenchimento 0–100 para a variante `progress`. @default 0
  final double pct;

  /// Recolore a barra ciano→âmbar→vermelho conforme `pct`. @default false
  final bool colorByZone;

  /// Instrumento renderizado à esquerda na variante `gauge`.
  final Widget? gauge;

  @override
  Widget build(BuildContext context) {
    return AppCard(
      child: switch (_variant) {
        _Variant.info => _info(),
        _Variant.gauge => _gauge(),
        _Variant.value || _Variant.progress => _valueOrProgress(),
      },
    );
  }

  Widget _info() {
    return Row(
      mainAxisAlignment: MainAxisAlignment.spaceBetween,
      children: [
        Flexible(
          child: Text(
            label,
            style: AppTypography.label.copyWith(color: AppColors.textTertiary),
            overflow: TextOverflow.ellipsis,
          ),
        ),
        const SizedBox(width: AppSpacing.s3),
        Text(
          '${value ?? '—'}',
          style: AppTypography.mono(
            const TextStyle(
              fontSize: 14,
              fontWeight: FontWeight.w600,
              color: AppColors.textPrimary,
            ),
          ),
        ),
      ],
    );
  }

  Widget _valueOrProgress() {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: [
        Text(label.toUpperCase(), style: AppTypography.overline),
        const SizedBox(height: AppSpacing.s2),
        _valueRow(),
        // O espaço da barra é sempre reservado para que `value` e `progress`
        // tenham a mesma altura; na variante `value` a barra fica invisível.
        const SizedBox(height: AppSpacing.s4),
        if (_variant == _Variant.progress)
          _ProgressBar(pct: pct, colorByZone: colorByZone)
        else
          const SizedBox(height: _ProgressBar.height),
      ],
    );
  }

  Widget _gauge() {
    return Row(
      children: [
        gauge!,
        const SizedBox(width: AppSpacing.s4),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            mainAxisSize: MainAxisSize.min,
            children: [
              _gaugeValue(),
              const SizedBox(height: AppSpacing.s2),
              Text(
                label,
                style: AppTypography.label
                    .copyWith(color: AppColors.textTertiary),
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
              ),
            ],
          ),
        ),
      ],
    );
  }

  /// Número + unidade para a variante `gauge`, onde o espaço horizontal é
  /// disputado com o instrumento. Um [Wrap] joga a unidade para a linha de
  /// baixo quando não cabe ao lado; o número trunca como último recurso (nunca
  /// estoura). O alinhamento por baixo aproxima a baseline do par.
  Widget _gaugeValue() {
    final number = Text(
      '${value ?? '—'}',
      maxLines: 1,
      overflow: TextOverflow.ellipsis,
      style: AppTypography.mono(
        const TextStyle(
          fontSize: 30,
          fontWeight: FontWeight.w700,
          height: 1,
          color: AppColors.textPrimary,
        ),
      ),
    );
    if (unit == null || unit!.isEmpty) return number;
    return Wrap(
      crossAxisAlignment: WrapCrossAlignment.end,
      spacing: 4,
      children: [
        number,
        Text(
          unit!,
          style: AppTypography.mono(
            const TextStyle(
              fontSize: 14,
              fontWeight: FontWeight.w500,
              color: AppColors.textTertiary,
            ),
          ),
        ),
      ],
    );
  }

  /// Número grande + sufixo de unidade, compartilhado por `value`/`progress`.
  Widget _valueRow() {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.baseline,
      textBaseline: TextBaseline.alphabetic,
      children: [
        Text(
          '${value ?? '—'}',
          style: AppTypography.mono(
            const TextStyle(
              fontSize: 30,
              fontWeight: FontWeight.w700,
              height: 1,
              color: AppColors.textPrimary,
            ),
          ),
        ),
        if (unit != null && unit!.isNotEmpty) ...[
          const SizedBox(width: 4),
          Text(
            unit!,
            style: AppTypography.mono(
              const TextStyle(
                fontSize: 14,
                fontWeight: FontWeight.w500,
                color: AppColors.textTertiary,
              ),
            ),
          ),
        ],
      ],
    );
  }
}

class _ProgressBar extends StatelessWidget {
  const _ProgressBar({required this.pct, required this.colorByZone});

  /// Altura da barra, em px. Também reservada (vazia) na variante `value`.
  static const double height = 6;

  final double pct;
  final bool colorByZone;

  @override
  Widget build(BuildContext context) {
    final fraction = (pct / 100).clamp(0.0, 1.0);
    final color = colorByZone
        ? (fraction > 0.9
            ? AppColors.red500
            : fraction > 0.78
                ? AppColors.amber500
                : AppColors.cyan500)
        : AppColors.cyan500;

    return ClipRRect(
      borderRadius: AppRadii.brPill,
      child: Container(
        height: height,
        color: AppColors.track,
        child: Align(
          alignment: Alignment.centerLeft,
          child: TweenAnimationBuilder<double>(
            tween: Tween<double>(end: fraction),
            duration: context.motion(AppMotion.durValue),
            curve: AppMotion.easeValue,
            builder: (context, f, _) => FractionallySizedBox(
              widthFactor: f,
              child: Container(color: color),
            ),
          ),
        ),
      ),
    );
  }
}
