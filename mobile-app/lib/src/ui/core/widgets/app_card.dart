import 'package:flutter/widgets.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';

/// A superfície base de todo card: fundo, borda hairline, raio e (opcional)
/// sombra. Centraliza a decoração antes copiada em cada card — `StatCard`,
/// `CardButton` e afins compõem este widget e variam só pelos parâmetros.
///
/// Os defaults reproduzem o card de dashboard (fundo [AppColors.surfaceCard],
/// raio [AppRadii.brLg], padding [AppSpacing.s5]); listas e botões sobrescrevem
/// `color`/`borderRadius`/`padding`/`constraints`. Sem sombra por padrão
/// ([boxShadow] nulo); informe-a quando quiser elevar o card. Com [onTap], a
/// superfície vira tocável (mesmo `HitTestBehavior.opaque` da lista).
class AppCard extends StatelessWidget {
  /// Cria a superfície de card que envolve [child].
  const AppCard({
    required this.child,
    this.color = AppColors.surfaceCard,
    this.borderRadius = AppRadii.brLg,
    this.padding = const EdgeInsets.all(AppSpacing.s5),
    this.boxShadow,
    this.constraints,
    this.onTap,
    super.key,
  });

  /// Conteúdo do card.
  final Widget child;

  /// Cor de fundo. @default [AppColors.surfaceCard]
  final Color color;

  /// Raio dos cantos. @default [AppRadii.brLg]
  final BorderRadius borderRadius;

  /// Espaçamento interno. @default `EdgeInsets.all(AppSpacing.s5)`
  final EdgeInsetsGeometry padding;

  /// Sombra opcional. @default `null` (sem sombra)
  final List<BoxShadow>? boxShadow;

  /// Restrições de tamanho (ex.: `minHeight`). @default `null`
  final BoxConstraints? constraints;

  /// Toque na superfície. Quando nulo, o card não é tocável.
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    final card = Container(
      constraints: constraints,
      padding: padding,
      decoration: BoxDecoration(
        color: color,
        borderRadius: borderRadius,
        border: Border.all(color: AppColors.borderHairline),
        boxShadow: boxShadow,
      ),
      child: child,
    );
    if (onTap == null) return card;
    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: card,
    );
  }
}
