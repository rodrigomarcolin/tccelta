import 'package:flutter/widgets.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/graph/sparkline_painter.dart';

/// Gráfico de linha minimalista (sparkline) de uma série temporal.
///
/// Curva suave + área esmaecida, sem eixos nem rótulos — pensado para viver
/// dentro de um card (ex.: `StatGraphCard`). A entrada é revelada da esquerda
/// para a direita, suavizando como os demais valores ao vivo do cockpit e
/// respeitando "reduzir movimento". Espelha o componente `Sparkline`.
class Sparkline extends StatelessWidget {
  /// Cria um sparkline para [values] (amostras em ordem cronológica).
  const Sparkline({
    required this.values,
    this.color = AppColors.cyan500,
    this.strokeWidth = 2.5,
    this.height = 120,
    super.key,
  });

  /// Amostras em ordem cronológica.
  final List<double> values;

  /// Cor do traço e base do gradiente. @default ciano
  final Color color;

  /// Espessura do traço. @default 2.5
  final double strokeWidth;

  /// Altura do gráfico em px. @default 120
  final double height;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      width: double.infinity,
      height: height,
      child: TweenAnimationBuilder<double>(
        // Reanima a revelação quando a série muda de comprimento.
        key: ValueKey(values.length),
        tween: Tween<double>(begin: 0, end: 1),
        duration: context.motion(AppMotion.durValue),
        curve: AppMotion.easeValue,
        builder: (context, progress, _) => CustomPaint(
          painter: SparklinePainter(
            values: values,
            color: color,
            strokeWidth: strokeWidth,
            fillProgress: progress,
          ),
        ),
      ),
    );
  }
}
