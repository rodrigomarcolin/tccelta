import 'dart:math' as math;

import 'package:flutter/rendering.dart';
import 'package:tccelta_mobile/src/core/theme/app_colors.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/gauge/gauge.dart';

/// Pinta a família de instrumentos [Gauge]. A geometria é portada 1:1 do
/// `Gauge.jsx` do design system (viewBox 200×200), escalada pelo tamanho do
/// canvas.
///
/// O número central e o label NÃO são pintados aqui — ficam sobrepostos como
/// widgets em [Gauge] para herdar a tipografia mono/tabular da marca.
class GaugePainter extends CustomPainter {
  /// Cria o painter para [variant] com [pct] e [color] resolvidos.
  const GaugePainter({
    required this.variant,
    required this.pct,
    required this.color,
    this.weight = Gauge.defaultWeight,
    this.warningThreshold = Gauge.defaultWarningThreshold,
    this.alertThreshold = Gauge.defaultAlertThreshold,
  });

  /// Formato do arco.
  final GaugeVariant variant;

  /// Fração preenchida 0..1 (já animada/clampada).
  final double pct;

  /// Cor do anel/ponteiro de progresso (já resolvida por zona, se aplicável).
  final Color color;

  /// Espessura do traço, em unidades do viewBox (200).
  final double weight;

  /// Início da zona de aviso (âmbar) das faixas de [GaugeVariant.arc180].
  final double warningThreshold;

  /// Início da zona de alerta (vermelho) das faixas de [GaugeVariant.arc180].
  final double alertThreshold;

  static const double _vb = 200; // viewBox de referência

  /// Raio que mantém a borda externa do traço fixa (~92) qualquer que seja o
  /// [weight], de modo que afinar o anel não o encolha.
  double _radius(double s) => (92 - weight / 2) * s;

  @override
  void paint(Canvas canvas, Size size) {
    final s = size.width / _vb; // fator de escala
    final center = Offset(100 * s, 100 * s);

    switch (variant) {
      case GaugeVariant.ring:
        _ring(canvas, center, r: _radius(s), stroke: weight * s, rounded: true);
      case GaugeVariant.arc270:
        _arc270(canvas, center, s);
      case GaugeVariant.arc180:
        _arc180(canvas, center, s);
    }
  }

  /// Anel de progresso fechado, começando no topo e girando no sentido horário.
  void _ring(
    Canvas canvas,
    Offset center, {
    required double r,
    required double stroke,
    required bool rounded,
  }) {
    final rect = Rect.fromCircle(center: center, radius: r);
    final trackPaint = Paint()
      ..style = PaintingStyle.stroke
      ..strokeWidth = stroke
      ..color = AppColors.track;
    canvas.drawCircle(center, r, trackPaint);

    if (pct <= 0) return;
    final progress = Paint()
      ..style = PaintingStyle.stroke
      ..strokeWidth = stroke
      ..strokeCap = rounded ? StrokeCap.round : StrokeCap.butt
      ..color = color;
    canvas.drawArc(rect, -math.pi / 2, 2 * math.pi * pct, false, progress);
  }

  /// Arco de 270° estilo velocímetro + marcas de escala.
  void _arc270(Canvas canvas, Offset center, double s) {
    const startAngle = 3 * math.pi / 4; // 135°
    const sweep = 3 * math.pi / 2; // 270°
    final r = _radius(s);
    final rect = Rect.fromCircle(center: center, radius: r);

    final track = Paint()
      ..style = PaintingStyle.stroke
      ..strokeWidth = weight * s
      ..strokeCap = StrokeCap.round
      ..color = AppColors.track;
    canvas.drawArc(rect, startAngle, sweep, false, track);

    if (pct > 0) {
      final progress = Paint()
        ..style = PaintingStyle.stroke
        ..strokeWidth = weight * s
        ..strokeCap = StrokeCap.round
        ..color = color;
      canvas.drawArc(rect, startAngle, sweep * pct, false, progress);
    }

    // Marcas (neutral-600), em coordenadas de viewBox.
    final tick = Paint()
      ..style = PaintingStyle.stroke
      ..strokeWidth = 2 * s
      ..color = AppColors.neutral600;
    const ticks = <List<double>>[
      [100, 20, 100, 30],
      [158, 44, 151, 51],
      [180, 100, 170, 100],
      [42, 44, 49, 51],
      [20, 100, 30, 100],
    ];
    for (final t in ticks) {
      canvas.drawLine(
        Offset(t[0] * s, t[1] * s),
        Offset(t[2] * s, t[3] * s),
        tick,
      );
    }
  }

  /// Arco de 180°+ com zonas fixas verde/âmbar/vermelho + ponteiro apontando o
  /// valor. As cores e a quantidade de zonas (3) são fixas; apenas os limites
  /// [warningThreshold]/[alertThreshold] variam.
  void _arc180(Canvas canvas, Offset center, double s) {
    final r = _radius(s);
    final rect = Rect.fromCircle(center: center, radius: r);

    // Faixa ativa: -110°..+110° em torno do topo (220° de varredura), a mesma
    // que o ponteiro percorre. O ponteiro e as zonas precisam compartilhar
    // exatamente este sistema angular.
    const startDeg = -110.0;
    const sweepDeg = 220.0;
    const startAngle = -math.pi / 2 + startDeg * math.pi / 180;
    const sweep = sweepDeg * math.pi / 180;

    final trackPaint = Paint()
      ..style = PaintingStyle.stroke
      ..strokeWidth = weight * s
      ..strokeCap = StrokeCap.round
      ..color = AppColors.track;
    canvas.drawArc(rect, startAngle, sweep, false, trackPaint);

    // Três faixas contíguas: verde até o aviso, âmbar até o alerta, vermelho
    // até o fim. Cada uma vai do limite da anterior até o seu [upTo], como
    // fração da varredura.
    final bands = <(double, Color)>[
      (warningThreshold, AppColors.green500),
      (alertThreshold, AppColors.amber500),
      (1, AppColors.red500),
    ];
    var fromPct = 0.0;
    for (final (upTo, bandColor) in bands) {
      final toPct = upTo.clamp(0.0, 1.0);
      if (toPct <= fromPct) continue;
      final paint = Paint()
        ..style = PaintingStyle.stroke
        ..strokeWidth = weight * s
        ..color = bandColor;
      canvas.drawArc(
        rect,
        startAngle + sweep * fromPct,
        sweep * (toPct - fromPct),
        false,
        paint,
      );
      fromPct = toPct;
    }

    // Ponteiro: ângulo -110°..+110° (0 = topo), comprimento (100-22)=78.
    final angleDeg = startDeg + sweepDeg * pct;
    final needleAngle = -math.pi / 2 + angleDeg * math.pi / 180;
    final tip = Offset(
      center.dx + 78 * s * math.cos(needleAngle),
      center.dy + 78 * s * math.sin(needleAngle),
    );
    final needle = Paint()
      ..style = PaintingStyle.stroke
      ..strokeWidth = 3 * s
      ..strokeCap = StrokeCap.round
      ..color = AppColors.textPrimary;
    canvas
      ..drawLine(center, tip, needle)
      // Hub central.
      ..drawCircle(
        center,
        6 * s,
        Paint()..color = AppColors.textPrimary,
      );
  }

  @override
  bool shouldRepaint(GaugePainter old) =>
      old.pct != pct ||
      old.color != color ||
      old.variant != variant ||
      old.weight != weight ||
      old.warningThreshold != warningThreshold ||
      old.alertThreshold != alertThreshold;
}
