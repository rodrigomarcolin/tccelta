import 'package:flutter/widgets.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/ui/core/icons/app_icon.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/app_card.dart';

/// Linha tocável de card: ícone à esquerda, título + subtítulo no meio, valor
/// à direita e um chevron fixo indicando que abre detalhe.
///
/// Construído sobre [AppCard] (fundo [AppColors.surfaceList], raio
/// [AppRadii.brMd]). O valor é renderizado em mono; quando nulo vira um traço.
/// [iconColor] tinge o [icon] — use [AppColors.cyan500] para destacá-lo.
class CardButton extends StatelessWidget {
  /// Cria um card-botão para [title]/[subtitle] com leitura [value].
  const CardButton({
    required this.icon,
    required this.title,
    required this.subtitle,
    this.value,
    this.unit,
    this.iconColor,
    this.onTap,
    super.key,
  });

  /// Ícone à esquerda.
  final AppIconData icon;

  /// Título legível (linha de cima).
  final String title;

  /// Subtítulo de apoio (linha de baixo, em mono).
  final String subtitle;

  /// Valor atual (já formatado). Nulo mostra um traço.
  final Object? value;

  /// Sufixo de unidade (ex.: "rpm", "°C"), renderizado mais fraco que o valor.
  final String? unit;

  /// Cor do [icon]. @default [AppColors.neutral600]
  final Color? iconColor;

  /// Toque no card (abre o detalhe).
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    return AppCard(
      color: AppColors.surfaceList,
      borderRadius: AppRadii.brMd,
      padding: const EdgeInsets.symmetric(
        horizontal: AppSpacing.s5,
        vertical: AppSpacing.s4,
      ),
      constraints: const BoxConstraints(minHeight: 56),
      onTap: onTap,
      child: Row(
        children: [
          AppIcon(
            icon,
            size: 18,
            color: iconColor ?? AppColors.neutral600,
          ),
          const SizedBox(width: AppSpacing.s4),
          Expanded(child: _titleAndSubtitle()),
          const SizedBox(width: AppSpacing.s3),
          _value(),
          const SizedBox(width: AppSpacing.s2),
          const AppIcon(
            AppIconData.chevron,
            size: 16,
            color: AppColors.neutral600,
          ),
        ],
      ),
    );
  }

  Widget _titleAndSubtitle() {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: [
        Text(
          title,
          style: AppTypography.label.copyWith(color: AppColors.textPrimary),
          overflow: TextOverflow.ellipsis,
        ),
        const SizedBox(height: 2),
        Text(
          subtitle,
          style: AppTypography.mono(
            const TextStyle(
              fontSize: 11,
              fontWeight: FontWeight.w500,
              color: AppColors.textTertiary,
            ),
          ),
        ),
      ],
    );
  }

  Widget _value() {
    if (value == null) {
      return Text(
        '—',
        style: AppTypography.mono(
          const TextStyle(
            fontSize: 16,
            fontWeight: FontWeight.w700,
            color: AppColors.neutral600,
          ),
        ),
      );
    }
    return Row(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.baseline,
      textBaseline: TextBaseline.alphabetic,
      children: [
        Text(
          '$value',
          style: AppTypography.mono(
            const TextStyle(
              fontSize: 16,
              fontWeight: FontWeight.w700,
              color: AppColors.textPrimary,
            ),
          ),
        ),
        if (unit != null && unit!.isNotEmpty) ...[
          const SizedBox(width: AppSpacing.s2),
          Text(
            unit!,
            style: AppTypography.mono(
              const TextStyle(
                fontSize: 11,
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
