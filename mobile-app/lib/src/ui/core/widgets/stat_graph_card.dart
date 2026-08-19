import 'package:flutter/widgets.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/graph/sparkline.dart';

/// Card de histórico de uma leitura: título, valor atual, sparkline e rodapé
/// com mín/méd/máx.
///
/// Recebe a série em [history] e **deriva** o valor atual (último ponto) e os
/// agregados mín/méd/máx, todos sobrescrevíveis. Os números são mono +
/// tabular, como no resto do cockpit. Espelha o componente `StatGraphCard`.
class StatGraphCard extends StatelessWidget {
  /// Cria o card para a série [history] rotulada por [label].
  const StatGraphCard({
    required this.label,
    required this.history,
    this.unit,
    this.value,
    this.min,
    this.avg,
    this.max,
    this.decimals = 1,
    super.key,
  });

  /// Rótulo da leitura, ex.: "Fluxo de ar (MAF)".
  final String label;

  /// Amostras em ordem cronológica.
  final List<double> history;

  /// Sufixo de unidade, ex.: "g/s".
  final String? unit;

  /// Valor atual (já formatado). @default último ponto de [history]
  final Object? value;

  /// Mínimo exibido. @default mínimo de [history]
  final num? min;

  /// Média exibida. @default média de [history]
  final num? avg;

  /// Máximo exibido. @default máximo de [history]
  final num? max;

  /// Casas decimais dos números derivados. @default 1
  final int decimals;

  String _fmt(num n) => n.toStringAsFixed(decimals);

  @override
  Widget build(BuildContext context) {
    final hasData = history.isNotEmpty;

    var min = this.min;
    var avg = this.avg;
    var max = this.max;
    if (hasData) {
      var lo = history.first;
      var hi = history.first;
      var sum = 0.0;
      for (final v in history) {
        if (v < lo) lo = v;
        if (v > hi) hi = v;
        sum += v;
      }
      min ??= lo;
      max ??= hi;
      avg ??= sum / history.length;
    }

    final bigValue = value?.toString() ?? (hasData ? _fmt(history.last) : '—');

    return DecoratedBox(
      decoration: BoxDecoration(
        color: AppColors.surfaceCard,
        borderRadius: AppRadii.brLg,
        border: Border.all(color: AppColors.borderHairline),
      ),
      child: Padding(
        padding: const EdgeInsets.all(AppSpacing.s5),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          mainAxisSize: MainAxisSize.min,
          children: [
            _header(bigValue),
            const SizedBox(height: AppSpacing.s5),
            if (hasData)
              Sparkline(values: history)
            else
              const SizedBox(height: 120),
            const SizedBox(height: AppSpacing.s4),
            _footer(min, avg, max),
          ],
        ),
      ),
    );
  }

  Widget _header(String bigValue) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Expanded(
          child: Text(
            label,
            style: AppTypography.bodyL.copyWith(fontWeight: FontWeight.w700),
            overflow: TextOverflow.ellipsis,
          ),
        ),
        const SizedBox(width: AppSpacing.s3),
        Row(
          crossAxisAlignment: CrossAxisAlignment.baseline,
          textBaseline: TextBaseline.alphabetic,
          children: [
            Text(
              bigValue,
              style: AppTypography.mono(
                const TextStyle(
                  fontSize: 28,
                  fontWeight: FontWeight.w700,
                  height: 1,
                  color: AppColors.cyan500,
                ),
              ),
            ),
            if (unit != null && unit!.isNotEmpty) ...[
              const SizedBox(width: 4),
              Text(
                unit!,
                style: AppTypography.mono(
                  const TextStyle(
                    fontSize: 13,
                    fontWeight: FontWeight.w500,
                    color: AppColors.textTertiary,
                  ),
                ),
              ),
            ],
          ],
        ),
      ],
    );
  }

  Widget _footer(num? min, num? avg, num? max) {
    return Row(
      mainAxisAlignment: MainAxisAlignment.spaceBetween,
      children: [
        _stat('mín', min),
        _stat('méd', avg),
        _stat('máx', max),
      ],
    );
  }

  Widget _stat(String label, num? value) {
    return Text.rich(
      TextSpan(
        children: [
          TextSpan(text: '$label '),
          TextSpan(
            text: value == null ? '—' : _fmt(value),
            style: AppTypography.mono(
              const TextStyle(
                fontSize: 12,
                fontWeight: FontWeight.w600,
                color: AppColors.textSecondary,
              ),
            ),
          ),
        ],
      ),
      style: AppTypography.mono(
        const TextStyle(
          fontSize: 12,
          fontWeight: FontWeight.w500,
          color: AppColors.textTertiary,
        ),
      ),
    );
  }
}
