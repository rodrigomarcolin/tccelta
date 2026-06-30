import 'package:flutter/widgets.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/ui/core/icons/app_icon.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/status_badge.dart';

/// Quadradinho arredondado e tonalizado contendo um [AppIcon] centralizado.
///
/// Elemento "leading" de linhas de lista (permissões, dispositivos): o fundo é
/// o wash do [tone] e o ícone, a cor sólida do tom. Reaproveita o mapa de cores
/// de [StatusToneX], a mesma fonte de [StatusBadge]. Para um tile neutro
/// (sem significado de cor), sobrescreva [background]/[iconColor].
class IconTile extends StatelessWidget {
  /// Cria um tile com [icon] no [tone] dado.
  const IconTile(
    this.icon, {
    this.tone = StatusTone.ok,
    this.size = 34,
    this.background,
    this.iconColor,
    super.key,
  });

  /// Ícone exibido no centro.
  final AppIconData icon;

  /// Tom semântico (define fundo e cor do ícone). @default [StatusTone.ok]
  final StatusTone tone;

  /// Lado do quadrado (px). @default 34
  final double size;

  /// Sobrescreve o fundo do tile. @default wash do [tone]
  final Color? background;

  /// Sobrescreve a cor do ícone. @default cor sólida do [tone]
  final Color? iconColor;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: size,
      height: size,
      decoration: BoxDecoration(
        color: background ?? tone.wash,
        borderRadius: AppRadii.brMd,
      ),
      alignment: Alignment.center,
      child: AppIcon(icon, size: size * 0.52, color: iconColor ?? tone.color),
    );
  }
}
