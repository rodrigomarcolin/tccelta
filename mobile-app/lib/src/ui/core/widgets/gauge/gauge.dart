import 'dart:math' as math;

import 'package:flutter/widgets.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/gauge/gauge_painter.dart';

/// Formato do arco do [Gauge]. Uma família, três formatos.
enum GaugeVariant {
  /// Anel de progresso fechado (padrão — limpo e legível de relance).
  ring,

  /// Arco clássico de velocímetro (270°) com marcas de escala.
  arc270,

  /// Arco de 180° com zonas fixas verde/âmbar/vermelho + ponteiro (semântica
  /// automotiva de temperatura).
  arc180,
}

/// O instrumento-assinatura do cockpit.
///
/// O [variant] define apenas o formato do arco: [GaugeVariant.ring] como gauge
/// padrão (RPM/velocidade), [GaugeVariant.arc270] para um clima de velocímetro
/// clássico e [GaugeVariant.arc180] para zonas fixas com ponteiro (temp.).
/// O [weight] define a espessura do traço — um traço fino deixa o número
/// central dominar (o antigo tratamento "number" é `ring` com [weight] baixo).
/// O texto do valor é mono + tabular e os anéis suavizam entre valores ao vivo
/// (não saltam).
class Gauge extends StatelessWidget {
  /// Cria um gauge para [value] sobre [max], no formato [variant].
  const Gauge({
    required this.value,
    this.max = 8000,
    this.label = 'RPM',
    this.unit = '',
    this.variant = GaugeVariant.ring,
    this.size = 178,
    this.weight = defaultWeight,
    this.colorByZone = true,
    this.warningThreshold = defaultWarningThreshold,
    this.alertThreshold = defaultAlertThreshold,
    this.invertZones = false,
    this.display,
    this.showValue = true,
    super.key,
  });

  /// Espessura padrão do traço, em unidades do viewBox (200). @default 13
  static const double defaultWeight = 13;

  /// Início padrão da zona de aviso (âmbar), como fração do fundo de escala.
  /// @default 0.78
  static const double defaultWarningThreshold = 0.78;

  /// Início padrão da zona de alerta (vermelho), como fração do fundo de
  /// escala. @default 0.9
  static const double defaultAlertThreshold = 0.9;

  /// Valor atual.
  final double value;

  /// Valor de fundo de escala. @default 8000
  final double max;

  /// Legenda central (ex.: "RPM"). @default "RPM"
  final String label;

  /// Sufixo de unidade anexado ao label / valor do ponteiro.
  final String unit;

  /// Formato do arco. @default [GaugeVariant.ring]
  final GaugeVariant variant;

  /// Diâmetro em px. @default 178
  final double size;

  /// Espessura do traço do arco, em unidades do viewBox (200). Valores baixos
  /// afinam o anel e ampliam o número central. @default [defaultWeight]
  final double weight;

  /// Recolore o traço de progresso (ciano→âmbar→vermelho) conforme cruza
  /// [warningThreshold] e [alertThreshold]. Não se aplica a
  /// [GaugeVariant.arc180], cujas zonas têm cor fixa. @default true
  final bool colorByZone;

  /// Início da zona de aviso (âmbar), como fração do fundo de escala (0..1).
  /// As cores e a quantidade de zonas (3) são fixas do componente.
  /// @default [defaultWarningThreshold]
  final double warningThreshold;

  /// Início da zona de alerta (vermelho), como fração do fundo de escala
  /// (0..1). @default [defaultAlertThreshold]
  final double alertThreshold;

  /// Inverte a semântica das zonas: com `true`, valor **alto** é a zona boa
  /// (ciano/verde) e valor **baixo** é a zona de atenção (vermelho) — para
  /// PIDs em que um valor alto é desejável (ex.: nível de combustível,
  /// tensão da bateria). @see `Obd2Pid.higherIsBetter`. @default false
  final bool invertZones;

  /// Sobrescreve o número central (ex.: string pré-formatada).
  final Object? display;

  /// Desenha o número + label (ou as zonas, no `arc180`) dentro do arco.
  /// `false` deixa só o instrumento — para um card pequeno que já mostra
  /// valor/label ao lado, evitando duplicar a informação. @default true
  final bool showValue;

  /// Cor do traço de progresso conforme o valor cruza os thresholds: ciano
  /// (normal) → âmbar (aviso) → vermelho (alerta). Com [invert] `true`, a
  /// ordem se inverte — zona alta vira ciano (boa) e zona baixa vira
  /// vermelho (atenção). @see [invertZones]
  static Color zoneColor(
    double pct,
    double warnAt,
    double alertAt, {
    bool invert = false,
  }) {
    final lowZone = pct < warnAt;
    final highZone = pct >= alertAt;
    if (!lowZone && !highZone) return AppColors.amber500;
    final isGoodZone = invert ? highZone : lowZone;
    return isGoodZone ? AppColors.cyan500 : AppColors.red500;
  }

  /// Milhar no padrão pt-BR ("3.240").
  static String formatPtBr(num n) {
    final digits = n.round().abs().toString();
    final buffer = StringBuffer(n < 0 ? '-' : '');
    for (var i = 0; i < digits.length; i++) {
      if (i != 0 && (digits.length - i) % 3 == 0) buffer.write('.');
      buffer.write(digits[i]);
    }
    return buffer.toString();
  }

  /// Fração 0..1 do tamanho da fonte do número central. Um traço mais fino
  /// (peso baixo) deixa o número dominar, recuperando o antigo visual "number".
  double get _numberFactor {
    final t = ((defaultWeight - weight) / (defaultWeight - 4)).clamp(0.0, 1.0);
    return 0.2 + 0.11 * t;
  }

  @override
  Widget build(BuildContext context) {
    final targetPct = (value / max).clamp(0.0, 1.0);
    final big = display?.toString() ?? formatPtBr(value);

    return SizedBox(
      width: size,
      height: size,
      child: TweenAnimationBuilder<double>(
        tween: Tween<double>(end: targetPct),
        duration: context.motion(AppMotion.durValue),
        curve: AppMotion.easeValue,
        builder: (context, pct, _) {
          final color = colorByZone
              ? zoneColor(
                  pct,
                  warningThreshold,
                  alertThreshold,
                  invert: invertZones,
                )
              : AppColors.cyan500;
          return Stack(
            alignment: Alignment.center,
            children: [
              CustomPaint(
                size: Size.square(size),
                painter: GaugePainter(
                  variant: variant,
                  pct: pct,
                  color: color,
                  weight: weight,
                  warningThreshold: warningThreshold,
                  alertThreshold: alertThreshold,
                  invertZones: invertZones,
                ),
              ),
              if (showValue)
                if (variant == GaugeVariant.arc180)
                  _zonesValue(big)
                else
                  _centerValue(big),
            ],
          );
        },
      ),
    );
  }

  /// Número central + label, para ring/arc270.
  Widget _centerValue(String big) {
    final numberSize = size * _numberFactor;
    final hasLabel = label.isNotEmpty;
    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        Text(
          big,
          style: AppTypography.mono(
            TextStyle(
              fontSize: numberSize,
              fontWeight: FontWeight.w700,
              height: 1,
              color: AppColors.textPrimary,
            ),
          ),
        ),
        if (hasLabel) ...[
          const SizedBox(height: 6),
          Text(
            unit.isNotEmpty ? '$label · $unit' : label,
            style: AppTypography.ui(
              const TextStyle(
                fontSize: 10,
                letterSpacing: 2,
                color: AppColors.textTertiary,
              ),
            ),
          ),
        ],
      ],
    );
  }

  /// Valor posicionado abaixo do centro, para a variante arc180.
  Widget _zonesValue(String big) {
    return Align(
      alignment: const Alignment(0, 0.45),
      child: Text(
        '$big$unit',
        style: AppTypography.mono(
          TextStyle(
            fontSize: math.max(18, size * 0.135),
            fontWeight: FontWeight.w700,
            height: 1,
            color: AppColors.textPrimary,
          ),
        ),
      ),
    );
  }
}
