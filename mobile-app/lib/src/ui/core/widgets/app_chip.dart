import 'package:flutter/widgets.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';

/// Pílula selecionável de uma linha de filtros/abas: rótulo + contagem
/// opcional, destacada em ciano quando [selected].
///
/// Extraído do padrão repetido nas abas de painel (`PanelTabsRow`) e nos
/// chips de componente da aba de Diagnóstico — mesma pill, mesma régua de
/// cor, só o conteúdo muda.
class AppChip extends StatelessWidget {
  /// Cria um chip com [label] e [selected] dados.
  const AppChip({
    required this.label,
    required this.selected,
    required this.onTap,
    this.trailingCount,
    this.trailingCountEmphasis = false,
    this.height = 40,
    super.key,
  });

  /// Texto principal do chip.
  final String label;

  /// Contagem opcional exibida após o [label] (ex.: nº de indicadores/DTCs).
  final String? trailingCount;

  /// Realça [trailingCount] com o tom de alerta (ex.: há códigos ativos),
  /// em vez do tratamento neutro/dimmed padrão. @default false
  final bool trailingCountEmphasis;

  /// Se o chip está selecionado (destaque ciano).
  final bool selected;

  /// Altura do chip. @default 40
  final double height;

  /// Chamado ao tocar o chip.
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final fg = selected ? AppColors.cyan500 : AppColors.textTertiary;
    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: Container(
        height: height,
        alignment: Alignment.center,
        padding: const EdgeInsets.symmetric(horizontal: AppSpacing.s5),
        decoration: BoxDecoration(
          color: selected ? AppColors.cyan14 : AppColors.surfaceList,
          borderRadius: AppRadii.brMd,
          border: Border.all(
            color: selected ? AppColors.cyan28 : AppColors.borderStrong,
          ),
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(
              label,
              style: AppTypography.ui(
                TextStyle(fontSize: 13, fontWeight: FontWeight.w600, color: fg),
              ),
            ),
            if (trailingCount != null) ...[
              const SizedBox(width: AppSpacing.s2),
              _count(fg),
            ],
          ],
        ),
      ),
    );
  }

  Widget _count(Color dimmedColor) {
    if (!trailingCountEmphasis) {
      return Text(
        trailingCount!,
        style: AppTypography.mono(
          TextStyle(
            fontSize: 11,
            fontWeight: FontWeight.w600,
            color: dimmedColor.withValues(alpha: 0.6),
          ),
        ),
      );
    }
    return Text(
      trailingCount!,
      style: AppTypography.mono(
        const TextStyle(
          fontSize: 10,
          fontWeight: FontWeight.w700,
          color: AppColors.red500,
        ),
      ),
    );
  }
}
