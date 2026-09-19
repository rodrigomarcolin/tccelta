import 'package:flutter/widgets.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';

/// Lista curta de itens de texto, cada um com um ponto marcador — para listas
/// informativas sem ordem/progresso (ex.: possíveis causas de uma falha).
///
/// Não confundir com `StepList`, que é um checklist de progresso
/// (feito/ativo/pendente): aqui todos os itens têm o mesmo peso visual.
class BulletList extends StatelessWidget {
  /// Cria a lista a partir de [items].
  const BulletList({required this.items, super.key});

  /// Textos dos itens, na ordem de exibição.
  final List<String> items;

  @override
  Widget build(BuildContext context) {
    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        for (var i = 0; i < items.length; i++) ...[
          if (i > 0) const SizedBox(height: AppSpacing.s2),
          _BulletItem(text: items[i]),
        ],
      ],
    );
  }
}

class _BulletItem extends StatelessWidget {
  const _BulletItem({required this.text});

  final String text;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(
        horizontal: AppSpacing.s4,
        vertical: AppSpacing.s3,
      ),
      decoration: BoxDecoration(
        color: AppColors.surfaceList,
        borderRadius: AppRadii.brMd,
        border: Border.all(color: AppColors.borderHairline),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Container(
            width: 5,
            height: 5,
            margin: const EdgeInsets.only(right: AppSpacing.s3, top: 7),
            decoration: const BoxDecoration(
              color: AppColors.textTertiary,
              shape: BoxShape.circle,
            ),
          ),
          Expanded(
            child: Text(
              text,
              style: AppTypography.body.copyWith(height: 1.4),
            ),
          ),
        ],
      ),
    );
  }
}
