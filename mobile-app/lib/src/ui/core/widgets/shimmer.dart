import 'package:flutter/widgets.dart';
import 'package:flutter_hooks/flutter_hooks.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/ui/core/hooks/hooks.dart';

/// Barra "esqueleto" com um brilho varrendo — o placeholder de carregamento do
/// design system.
///
/// Usada enquanto um dado ainda não chegou (ex.: os PIDs do painel antes da
/// primeira leitura). O destaque é **neutro, não ciano** — ciano é reservado
/// para "ao vivo", então um shimmer neutro comunica "ainda inicializando" sem
/// se confundir com um valor vivo. Sob "reduzir movimento" do SO a varredura
/// congela e resta uma barra estática em [AppColors.track] (via
/// [useLoopController]).
class Shimmer extends HookWidget {
  /// Cria a barra de shimmer. [width] nulo preenche a largura disponível.
  const Shimmer({
    this.width,
    this.height = 12,
    this.borderRadius = AppRadii.brSm,
    super.key,
  });

  /// Largura em px. `null` = preenche a largura disponível (`double.infinity`).
  final double? width;

  /// Altura da barra em px. @default 12
  final double height;

  /// Raio dos cantos. @default [AppRadii.brSm]
  final BorderRadius borderRadius;

  @override
  Widget build(BuildContext context) {
    final controller = useLoopController(AppMotion.durShimmer);
    return AnimatedBuilder(
      animation: controller,
      builder: (context, _) {
        // O destaque entra pela esquerda e sai pela direita (-1.5 → 1.5 da
        // largura). Congelado em 0 sob "reduzir movimento" → barra chapada.
        final slide = -1.5 + 3 * controller.value;
        return Container(
          width: width ?? double.infinity,
          height: height,
          decoration: BoxDecoration(
            borderRadius: borderRadius,
            gradient: LinearGradient(
              colors: const [
                AppColors.track,
                AppColors.neutral700,
                AppColors.track,
              ],
              stops: const [0.15, 0.5, 0.85],
              transform: _ShimmerSlide(slide),
            ),
          ),
        );
      },
    );
  }
}

/// Desliza o gradiente do shimmer horizontalmente por uma fração da largura do
/// retângulo — é o que faz o brilho "varrer".
class _ShimmerSlide extends GradientTransform {
  const _ShimmerSlide(this.slidePercent);

  /// Deslocamento horizontal como fração da largura.
  final double slidePercent;

  @override
  Matrix4? transform(Rect bounds, {TextDirection? textDirection}) =>
      Matrix4.translationValues(bounds.width * slidePercent, 0, 0);
}
