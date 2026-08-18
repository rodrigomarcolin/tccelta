import 'package:flutter/widgets.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/app_card.dart';

/// Linha "rótulo + subtítulo" à esquerda, `−`/valor/`+` à direita, para
/// editar um número inteiro dentro de [min]..[max] em passos de [step].
///
/// Único átomo por trás de todo campo numérico da tela de formato do
/// indicador — valor mínimo/máximo da escala, limites baixo/médio do gauge
/// ponteiro e quantidade de pontos do histórico são todos um [NumberStepper].
class NumberStepper extends StatelessWidget {
  /// Cria o stepper para [value], clampado em [min]..[max].
  const NumberStepper({
    required this.label,
    required this.value,
    required this.min,
    required this.max,
    required this.onChanged,
    this.sublabel,
    this.step = 1,
    this.formatValue,
    super.key,
  });

  /// Rótulo principal (ex.: "Valor máximo").
  final String label;

  /// Detalhe discreto abaixo do rótulo (ex.: "topo da escala · rpm").
  final String? sublabel;

  /// Valor atual — sempre mantido dentro de [min]..[max] pelo chamador.
  final double value;

  /// Piso do valor (o `−` some/desativa ao alcançá-lo).
  final double min;

  /// Teto do valor (o `+` some/desativa ao alcançá-lo).
  final double max;

  /// Incremento por toque. @default 1
  final double step;

  /// Chamado com o novo valor já clampado em [min]..[max].
  final ValueChanged<double> onChanged;

  /// Formata [value] para exibição. @default arredonda para inteiro
  final String Function(double value)? formatValue;

  void _bump(double dir) {
    final next = (value + dir * step).clamp(min, max);
    if (next != value) onChanged(next);
  }

  @override
  Widget build(BuildContext context) {
    final display = formatValue?.call(value) ?? value.round().toString();
    return AppCard(
      color: AppColors.surfaceSunken,
      padding: const EdgeInsets.symmetric(
        horizontal: AppSpacing.s5,
        vertical: AppSpacing.s4,
      ),
      child: Row(
        children: [
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              mainAxisSize: MainAxisSize.min,
              children: [
                Text(label, style: AppTypography.label),
                if (sublabel != null) ...[
                  const SizedBox(height: 2),
                  Text(
                    sublabel!,
                    style: AppTypography.body.copyWith(
                      fontSize: 11,
                      color: AppColors.textTertiary,
                    ),
                  ),
                ],
              ],
            ),
          ),
          const SizedBox(width: AppSpacing.s3),
          _StepButton(symbol: '−', onTap: value > min ? () => _bump(-1) : null),
          SizedBox(
            width: 64,
            child: Text(
              display,
              textAlign: TextAlign.center,
              style: AppTypography.mono(
                const TextStyle(fontSize: 16, fontWeight: FontWeight.w700),
              ),
            ),
          ),
          _StepButton(symbol: '+', onTap: value < max ? () => _bump(1) : null),
        ],
      ),
    );
  }
}

class _StepButton extends StatelessWidget {
  const _StepButton({required this.symbol, required this.onTap});

  final String symbol;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    final enabled = onTap != null;
    return AppCard(
      color: AppColors.surfaceList,
      borderRadius: AppRadii.brSm,
      padding: EdgeInsets.zero,
      constraints: const BoxConstraints.tightFor(width: 36, height: 36),
      onTap: onTap,
      child: Center(
        child: Text(
          symbol,
          style: AppTypography.ui(
            TextStyle(
              fontSize: 20,
              fontWeight: FontWeight.w700,
              color: enabled ? AppColors.textPrimary : AppColors.textTertiary,
              height: 1,
            ),
          ),
        ),
      ),
    );
  }
}
