import 'package:flutter/widgets.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/app_card.dart';

/// Alternador de segmentos lado a lado, um ativo em ciano por vez.
///
/// Genérico em [T] — hoje usado só para o tamanho do gauge (Menor/Maior),
/// mas sem nenhuma premissa de domínio.
class SegmentedToggle<T> extends StatelessWidget {
  /// Cria o alternador entre [options], com [value] destacado.
  const SegmentedToggle({
    required this.options,
    required this.value,
    required this.labelOf,
    required this.onChanged,
    super.key,
  });

  /// Segmentos disponíveis, na ordem de exibição.
  final List<T> options;

  /// Segmento atualmente selecionado.
  final T value;

  /// Rótulo exibido para cada segmento.
  final String Function(T option) labelOf;

  /// Chamado com o segmento tocado (mesmo se já selecionado).
  final ValueChanged<T> onChanged;

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        for (var i = 0; i < options.length; i++) ...[
          if (i > 0) const SizedBox(width: AppSpacing.s2),
          Expanded(child: _segment(options[i])),
        ],
      ],
    );
  }

  Widget _segment(T option) {
    final selected = option == value;
    return AppCard(
      color: selected ? AppColors.cyan500 : AppColors.surfaceSunken,
      borderRadius: AppRadii.brMd,
      border: selected ? null : Border.all(color: AppColors.borderHairline),
      padding: const EdgeInsets.symmetric(vertical: AppSpacing.s4),
      onTap: () => onChanged(option),
      child: Center(
        child: Text(
          labelOf(option),
          style: AppTypography.ui(
            TextStyle(
              fontSize: 13,
              fontWeight: FontWeight.w600,
              color: selected ? AppColors.accentOn : AppColors.textSecondary,
            ),
          ),
        ),
      ),
    );
  }
}
