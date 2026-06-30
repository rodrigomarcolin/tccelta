import 'package:flutter/widgets.dart';
import 'package:tccelta_mobile/src/core/theme/app_colors.dart';

/// OBD2 Cockpit — elevação (sombras).
///
/// Profundidade é rara. Só a moldura do device ganha drop shadow de verdade;
/// superfícies usam um hairline interno de 1px. Espelha `tokens/effects.css`.
abstract final class AppShadows {
  AppShadows._();

  /// Elevação — somente a moldura do device.
  /// CSS: `0 30px 70px -20px rgba(0,0,0,.7)` (+ anel de 1px, ver borda).
  static const List<BoxShadow> device = [
    BoxShadow(
      color: Color(0xB3000000), // rgba(0,0,0,.7)
      offset: Offset(0, 30),
      blurRadius: 70,
      spreadRadius: -20,
    ),
  ];
}

/// OBD2 Cockpit — glows de acento ("ao vivo").
///
/// O glow ciano sinaliza "ao vivo": `0 0 8px` num dot de status,
/// `0 0 14px` no centro do radar de scan.
abstract final class AppGlows {
  AppGlows._();

  /// Dot de status — `0 0 8px var(--cyan-500)`.
  static const List<BoxShadow> cyan = [
    BoxShadow(color: AppColors.cyan500, blurRadius: 8),
  ];

  /// Radar / centro de scan — `0 0 14px var(--cyan-500)`.
  static const List<BoxShadow> cyanLg = [
    BoxShadow(color: AppColors.cyan500, blurRadius: 14),
  ];
}

/// OBD2 Cockpit — movimento.
///
/// Restrito e físico. Valores ao vivo *suavizam* para o lugar (não saltam);
/// dots "respiram"; spinners giram. Espelha `tokens/effects.css`.
abstract final class AppMotion {
  AppMotion._();

  /// Transição de anel / barra / número (live value).
  static const Duration durValue = Duration(milliseconds: 550);

  /// Crossfade de cor de zona.
  static const Duration durColor = Duration(milliseconds: 400);

  /// Dot "respirando" ao vivo.
  static const Duration durPulse = Duration(seconds: 2);

  /// Spinner de conexão.
  static const Duration durSpin = Duration(milliseconds: 1100);

  /// Easing de assentamento do valor — `cubic-bezier(.25,.1,.25,1)`.
  static const Cubic easeValue = Cubic(0.25, 0.1, 0.25, 1);
}

/// Acesso a movimento sensível à acessibilidade.
///
/// Ponto único de verdade para a preferência "reduzir movimento" do SO
/// ([MediaQuery.disableAnimationsOf]). Os componentes consultam estes acessores
/// em vez de repetir a checagem — quem decide se anima é o tema, não o widget.
extension MotionX on BuildContext {
  /// `true` quando o usuário pediu para reduzir animações no sistema.
  bool get reduceMotion => MediaQuery.disableAnimationsOf(this);

  /// [full] normalmente, ou [Duration.zero] sob "reduzir movimento" — fazendo
  /// animações implícitas (`Animated*`, `TweenAnimationBuilder`) saltarem
  /// direto ao valor-alvo em vez de interpolar.
  Duration motion(Duration full) => reduceMotion ? Duration.zero : full;
}
