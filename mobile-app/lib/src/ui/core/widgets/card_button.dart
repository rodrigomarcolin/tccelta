import 'package:flutter/widgets.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/ui/core/icons/app_icon.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/app_card.dart';

/// Linha de lista tocável genérica: um elemento à esquerda, título + subtítulo
/// no meio e, opcionalmente, um valor e/ou chevron à direita.
///
/// Construída sobre [AppCard]. Por padrão é a "linha de sensor" do painel —
/// ícone bare à esquerda, título legível, subtítulo mono, valor mono à direita
/// e chevron de detalhe. Os parâmetros opcionais a generalizam para outras
/// listas (ex.: permissões, seleção de dispositivos) sem mudar esse padrão:
///
/// - [leading] troca o ícone bare por outro widget (ex.: `IconTile`).
/// - [titleMono] mostra o título em mono (nome de dispositivo).
/// - [subtitleColor]/[subtitleMono] ajustam a cor/fonte do subtítulo.
/// - [showValue] esconde a coluna de valor.
/// - [trailing] troca o chevron por outro widget à direita (ex.: um
///   alternador de ação); ignora [showChevron] quando informado.
/// - [selected] destaca a borda em ciano; [dimmed] esmaece a linha.
class CardButton extends StatelessWidget {
  /// Cria uma linha de lista para [title]/[subtitle].
  ///
  /// [icon]/[leading] são opcionais: sem nenhum dos dois, a linha não tem
  /// elemento à esquerda (título/subtítulo encostam na margem).
  const CardButton({
    required this.title,
    required this.subtitle,
    this.icon,
    this.value,
    this.unit,
    this.iconColor,
    this.leading,
    this.trailing,
    this.titleMono = false,
    this.subtitleColor,
    this.subtitleMono = true,
    this.showValue = true,
    this.showChevron = true,
    this.selected = false,
    this.dimmed = false,
    this.onTap,
    super.key,
  });

  /// Ícone bare à esquerda. Ignorado quando [leading] é informado.
  final AppIconData? icon;

  /// Título legível (linha de cima).
  final String title;

  /// Subtítulo de apoio (linha de baixo).
  final String subtitle;

  /// Valor atual (já formatado). Nulo mostra um traço (se [showValue]).
  final Object? value;

  /// Sufixo de unidade (ex.: "rpm", "°C"), renderizado mais fraco que o valor.
  final String? unit;

  /// Cor do [icon] bare. @default [AppColors.neutral600]
  final Color? iconColor;

  /// Elemento à esquerda no lugar do [icon] bare (ex.: `IconTile`).
  final Widget? leading;

  /// Elemento à direita no lugar do chevron (ex.: um alternador de ação).
  /// Quando informado, ignora [showChevron].
  final Widget? trailing;

  /// Renderiza o título em mono (nome de dispositivo). @default false
  final bool titleMono;

  /// Cor do subtítulo. @default [AppColors.textTertiary]
  final Color? subtitleColor;

  /// Subtítulo em mono (true) ou sans (false). @default true
  final bool subtitleMono;

  /// Mostra a coluna de valor (traço quando [value] é nulo). @default true
  final bool showValue;

  /// Mostra o chevron de detalhe à direita. @default true
  final bool showChevron;

  /// Destaca a borda em ciano (item selecionado). @default false
  final bool selected;

  /// Esmaece a linha (item indisponível/fraco). @default false
  final bool dimmed;

  /// Toque na linha (abre o detalhe / seleciona).
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    final leadingWidget =
        leading ??
        (icon != null
            ? AppIcon(icon!, size: 18, color: iconColor ?? AppColors.neutral600)
            : null);
    final card = AppCard(
      color: selected ? AppColors.surfaceCard : AppColors.surfaceList,
      borderRadius: AppRadii.brMd,
      border: selected ? Border.all(color: AppColors.cyan28) : null,
      padding: const EdgeInsets.symmetric(
        horizontal: AppSpacing.s5,
        vertical: AppSpacing.s4,
      ),
      constraints: const BoxConstraints(minHeight: 56),
      onTap: onTap,
      child: Row(
        children: [
          if (leadingWidget != null) ...[
            leadingWidget,
            const SizedBox(width: AppSpacing.s4),
          ],
          Expanded(child: _titleAndSubtitle()),
          if (showValue) ...[
            const SizedBox(width: AppSpacing.s3),
            _value(),
          ],
          if (trailing != null) ...[
            const SizedBox(width: AppSpacing.s2),
            trailing!,
          ] else if (showChevron) ...[
            const SizedBox(width: AppSpacing.s2),
            const AppIcon(
              AppIconData.chevron,
              size: 16,
              color: AppColors.neutral600,
            ),
          ],
        ],
      ),
    );
    if (!dimmed) return card;
    return Opacity(opacity: 0.55, child: card);
  }

  Widget _titleAndSubtitle() {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: [
        _title(),
        const SizedBox(height: 2),
        Text(subtitle, style: _subtitleStyle()),
      ],
    );
  }

  Widget _title() {
    final style = titleMono
        ? AppTypography.mono(
            const TextStyle(
              fontSize: 15,
              fontWeight: FontWeight.w600,
              color: AppColors.textPrimary,
            ),
          )
        : AppTypography.label.copyWith(color: AppColors.textPrimary);
    return Text(title, style: style, overflow: TextOverflow.ellipsis);
  }

  TextStyle _subtitleStyle() {
    final color = subtitleColor ?? AppColors.textTertiary;
    final base = TextStyle(
      fontSize: subtitleMono ? 11 : 12,
      fontWeight: FontWeight.w500,
      color: color,
    );
    return subtitleMono ? AppTypography.mono(base) : AppTypography.ui(base);
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
