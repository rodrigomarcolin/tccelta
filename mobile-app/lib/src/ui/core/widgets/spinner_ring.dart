import 'dart:math' as math;

import 'package:flutter/widgets.dart';
import 'package:flutter_hooks/flutter_hooks.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/ui/core/hooks/hooks.dart';

/// Anel de progresso indeterminado: trilha escura + um arco ciano girando,
/// com um [label] opcional no centro.
///
/// Átomo genérico do design system. É o indicador de "algo em andamento"
/// circular do app (ex.: o spinner "BLE" da tela de conexão). A rotação usa
/// [useLoopController], então congela sozinho sob "reduzir movimento" do SO —
/// mesma ponte de acessibilidade do `RadarScanner` e do mini-spinner do
/// `StepList`.
///
/// Os defaults reproduzem o spinner de conexão; ajuste [size], [sweep],
/// cores e espessura para outros contextos.
class SpinnerRing extends HookWidget {
  /// Cria o anel com diâmetro [size] e um [label] central opcional.
  const SpinnerRing({
    this.size = 120,
    this.label,
    this.strokeWidth = 4,
    this.sweep = math.pi / 2.2,
    this.trackColor = AppColors.neutral800,
    this.arcColor = AppColors.cyan500,
    this.labelColor = AppColors.cyan500,
    this.duration = AppMotion.durSpin,
    this.active = true,
    super.key,
  });

  /// Diâmetro do anel (px). @default 120
  final double size;

  /// Texto discreto no centro (ex.: `'BLE'`). @default null (sem rótulo)
  final String? label;

  /// Espessura da trilha e do arco (px). @default 4
  final double strokeWidth;

  /// Ângulo do arco em radianos, a partir do topo. @default ~82° (`pi / 2.2`)
  final double sweep;

  /// Cor da trilha de fundo. @default [AppColors.neutral800]
  final Color trackColor;

  /// Cor do arco que gira. @default [AppColors.cyan500]
  final Color arcColor;

  /// Cor do [label] central. @default [AppColors.cyan500]
  final Color labelColor;

  /// Duração de uma volta completa. @default [AppMotion.durSpin]
  final Duration duration;

  /// Se o arco deve girar. Quando `false`, o anel congela estático.
  /// @default true
  final bool active;

  @override
  Widget build(BuildContext context) {
    final controller = useLoopController(duration, active: active);
    return SizedBox(
      width: size,
      height: size,
      child: Stack(
        alignment: Alignment.center,
        children: [
          RotationTransition(
            turns: controller,
            child: CustomPaint(
              size: Size(size, size),
              painter: _RingPainter(
                strokeWidth: strokeWidth,
                sweep: sweep,
                trackColor: trackColor,
                arcColor: arcColor,
              ),
            ),
          ),
          if (label != null && label!.isNotEmpty)
            Text(
              label!,
              style: AppTypography.mono(
                TextStyle(
                  fontSize: 13,
                  fontWeight: FontWeight.w600,
                  color: labelColor,
                ),
              ),
            ),
        ],
      ),
    );
  }
}

/// Trilha escura + um arco ciano (o "progresso" girando).
class _RingPainter extends CustomPainter {
  const _RingPainter({
    required this.strokeWidth,
    required this.sweep,
    required this.trackColor,
    required this.arcColor,
  });

  final double strokeWidth;
  final double sweep;
  final Color trackColor;
  final Color arcColor;

  @override
  void paint(Canvas canvas, Size size) {
    final center = size.center(Offset.zero);
    final radius = size.width / 2 - strokeWidth;
    final rect = Rect.fromCircle(center: center, radius: radius);

    final track = Paint()
      ..style = PaintingStyle.stroke
      ..strokeWidth = strokeWidth
      ..color = trackColor;
    canvas.drawCircle(center, radius, track);

    final arc = Paint()
      ..style = PaintingStyle.stroke
      ..strokeWidth = strokeWidth
      ..strokeCap = StrokeCap.round
      ..color = arcColor;
    // Arco a partir do topo.
    canvas.drawArc(rect, -math.pi / 2, sweep, false, arc);
  }

  @override
  bool shouldRepaint(_RingPainter oldDelegate) =>
      oldDelegate.strokeWidth != strokeWidth ||
      oldDelegate.sweep != sweep ||
      oldDelegate.trackColor != trackColor ||
      oldDelegate.arcColor != arcColor;
}
