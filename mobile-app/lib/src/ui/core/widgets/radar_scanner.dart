import 'package:flutter/widgets.dart';
import 'package:flutter_hooks/flutter_hooks.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/ui/core/hooks/hooks.dart';

/// Radar de busca BLE animado: anéis concêntricos, uma varredura cônica girando
/// e um ponto central ciano com glow.
///
/// Usado na tela de scan enquanto procura dispositivos. A varredura congela sob
/// "reduzir movimento" do SO ou quando [active] é `false` (ex.: scan
/// encerrado); os anéis e o centro permanecem.
class RadarScanner extends HookWidget {
  /// Cria o radar com diâmetro [size].
  const RadarScanner({this.size = 170, this.active = true, super.key});

  /// Diâmetro do anel externo (px). @default 170
  final double size;

  /// Se a varredura deve girar. Quando `false`, o radar congela estático.
  /// @default true
  final bool active;

  @override
  Widget build(BuildContext context) {
    // Um pouco mais lento que o spinner — uma varredura de radar "calma".
    final controller = useLoopController(
      const Duration(milliseconds: 2400),
      active: active,
    );
    final size = this.size;
    return SizedBox(
      width: size,
      height: size,
      child: Stack(
        alignment: Alignment.center,
        children: [
          _ring(size, AppColors.cyan10),
          _ring(size * 0.67, AppColors.cyan14),
          _ring(size * 0.34, AppColors.cyan28),
          // Varredura cônica girando.
          ClipOval(
            child: RotationTransition(
              turns: controller,
              child: Container(
                width: size,
                height: size,
                decoration: const BoxDecoration(
                  shape: BoxShape.circle,
                  gradient: SweepGradient(
                    colors: [
                      Color(0x0022D3EE),
                      Color(0x0022D3EE),
                      AppColors.cyan28,
                    ],
                    stops: [0, 0.78, 1],
                  ),
                ),
              ),
            ),
          ),
          // Ponto central com glow.
          Container(
            width: 14,
            height: 14,
            decoration: const BoxDecoration(
              color: AppColors.cyan500,
              shape: BoxShape.circle,
              boxShadow: AppGlows.cyanLg,
            ),
          ),
        ],
      ),
    );
  }

  Widget _ring(double diameter, Color color) {
    return Container(
      width: diameter,
      height: diameter,
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        border: Border.all(color: color),
      ),
    );
  }
}
