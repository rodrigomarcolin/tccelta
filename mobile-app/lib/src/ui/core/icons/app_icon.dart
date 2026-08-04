import 'package:flutter/widgets.dart';
import 'package:flutter_svg/flutter_svg.dart';
import 'package:tccelta_mobile/src/core/theme/app_colors.dart';
import 'package:tccelta_mobile/src/ui/core/icons/app_icons.dart';

export 'app_icons.dart';

/// Átomo de ícone do design system.
///
/// Renderiza um dos [AppIconData] (SVG outline customizado) num tamanho e cor
/// parametrizáveis. A cor é aplicada via `srcIn`, então preserva a opacidade
/// interna do traço (ex.: o anel externo do ícone `sinal`).
///
/// A cor padrão é [AppColors.textPrimary]; o chamador escolhe a cor de acordo
/// com o contexto (ativo = [AppColors.cyan500], inativo =
/// [AppColors.neutral400], destrutivo = [AppColors.red500]) — conforme a marca.
class AppIcon extends StatelessWidget {
  /// Cria um ícone para [icon] com [size] e [color] opcionais.
  const AppIcon(
    this.icon, {
    this.size = 22,
    this.color,
    this.semanticLabel,
    super.key,
  });

  /// Ícone a renderizar.
  final AppIconData icon;

  /// Lado do quadrado (px). @default 22
  final double size;

  /// Cor do traço/preenchimento. @default [AppColors.textPrimary]
  final Color? color;

  /// Rótulo de acessibilidade.
  final String? semanticLabel;

  @override
  Widget build(BuildContext context) {
    return SvgPicture.asset(
      icon.asset,
      width: size,
      height: size,
      colorFilter: ColorFilter.mode(
        color ?? AppColors.textPrimary,
        BlendMode.srcIn,
      ),
      semanticsLabel: semanticLabel,
    );
  }
}
