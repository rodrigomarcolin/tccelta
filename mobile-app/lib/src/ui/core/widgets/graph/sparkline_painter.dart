import 'package:flutter/rendering.dart';
import 'package:tccelta_mobile/src/core/theme/app_colors.dart';

/// Pinta uma série temporal como uma curva suave + área preenchida (sparkline).
///
/// A geometria é portada do design system: x distribuído uniformemente na
/// largura e y normalizado pelo min/max da série (com um respiro vertical para
/// a curva não encostar nas bordas). A suavização usa interpolação
/// **Catmull-Rom convertida em Bézier cúbica** (tensão 1/6), o mesmo esquema
/// que dá o traço "respirado" do mock. O número/valor NÃO é pintado aqui —
/// fica como widget sobreposto em `StatGraphCard`, herdando a tipografia mono.
class SparklinePainter extends CustomPainter {
  /// Cria o painter para [values] com [color] e [strokeWidth] resolvidos.
  const SparklinePainter({
    required this.values,
    this.color = AppColors.cyan500,
    this.strokeWidth = 2.5,
    this.fillProgress = 1,
  });

  /// Amostras em ordem cronológica.
  final List<double> values;

  /// Cor do traço e base do gradiente da área.
  final Color color;

  /// Espessura do traço. @default 2.5
  final double strokeWidth;

  /// Fração revelada 0..1 (esquerda → direita), para a entrada animada.
  /// @default 1
  final double fillProgress;

  /// Respiro vertical (fração da altura) reservado em cima e embaixo, para a
  /// curva não tocar as bordas do canvas.
  static const double _padFraction = 0.12;

  @override
  void paint(Canvas canvas, Size size) {
    if (values.length < 2) {
      // 0 ou 1 ponto: nada de curva. Com 1 ponto, marca-o no centro vertical.
      if (values.length == 1) {
        canvas.drawCircle(
          Offset(size.width / 2, size.height / 2),
          strokeWidth,
          Paint()..color = color,
        );
      }
      return;
    }

    final points = _mapPoints(size);

    if (fillProgress < 1) {
      canvas
        ..save()
        ..clipRect(
          Rect.fromLTWH(
            0,
            0,
            size.width * fillProgress.clamp(0.0, 1.0),
            size.height,
          ),
        );
    }

    final line = _smoothPath(points);

    // Área: fecha o traço até a base e preenche com um gradiente que esmaece.
    final fill = Path.from(line)
      ..lineTo(points.last.dx, size.height)
      ..lineTo(points.first.dx, size.height)
      ..close();
    final fillPaint = Paint()
      ..shader = LinearGradient(
        begin: Alignment.topCenter,
        end: Alignment.bottomCenter,
        colors: [
          color.withValues(alpha: 0.18),
          color.withValues(alpha: 0),
        ],
      ).createShader(Offset.zero & size);
    canvas.drawPath(fill, fillPaint);

    // Traço por cima.
    final stroke = Paint()
      ..style = PaintingStyle.stroke
      ..strokeWidth = strokeWidth
      ..strokeCap = StrokeCap.round
      ..strokeJoin = StrokeJoin.round
      ..color = color;
    canvas.drawPath(line, stroke);

    if (fillProgress < 1) canvas.restore();
  }

  /// Mapeia as amostras para pontos no canvas (x uniforme, y normalizado).
  List<Offset> _mapPoints(Size size) {
    var min = values.first;
    var max = values.first;
    for (final v in values) {
      if (v < min) min = v;
      if (v > max) max = v;
    }
    final pad = size.height * _padFraction;
    final usableH = size.height - pad * 2;
    final span = max - min;
    final stepX = size.width / (values.length - 1);

    return [
      for (var i = 0; i < values.length; i++)
        Offset(
          stepX * i,
          // Série constante (span == 0): linha reta no meio. Senão, inverte o
          // eixo (maior valor = mais alto = y menor).
          span == 0
              ? size.height / 2
              : pad + usableH * (1 - (values[i] - min) / span),
        ),
    ];
  }

  /// Curva suave por Catmull-Rom → Bézier cúbica (tensão 1/6), com os vizinhos
  /// clampados nas pontas.
  Path _smoothPath(List<Offset> p) {
    final path = Path()..moveTo(p.first.dx, p.first.dy);
    for (var i = 0; i < p.length - 1; i++) {
      final p0 = i == 0 ? p[i] : p[i - 1];
      final p1 = p[i];
      final p2 = p[i + 1];
      final p3 = i + 2 < p.length ? p[i + 2] : p2;
      final cp1 = p1 + (p2 - p0) / 6;
      final cp2 = p2 - (p3 - p1) / 6;
      path.cubicTo(cp1.dx, cp1.dy, cp2.dx, cp2.dy, p2.dx, p2.dy);
    }
    return path;
  }

  @override
  bool shouldRepaint(SparklinePainter old) =>
      old.values != values ||
      old.color != color ||
      old.strokeWidth != strokeWidth ||
      old.fillProgress != fillProgress;
}
