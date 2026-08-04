import 'package:flutter/painting.dart';
import 'package:google_fonts/google_fonts.dart';
import 'package:tccelta_mobile/src/core/theme/app_colors.dart';

/// OBD2 Cockpit — tokens de tipografia.
///
/// Duas famílias: **Space Grotesk** para a interface (títulos, labels, corpo)
/// e **JetBrains Mono** para TODOS os dados (valores, PIDs, hora, endereços).
/// Dados numéricos sempre usam [FontFeature.tabularFigures] para que os dígitos
/// não "tremam" enquanto valores ao vivo mudam.
///
/// Espelha `tokens/typography.css` do design system.
abstract final class AppTypography {
  AppTypography._();

  /// Numerais tabulares — regra central da marca para qualquer dado.
  static const List<FontFeature> _tabular = [FontFeature.tabularFigures()];

  /// Família de interface (Space Grotesk).
  static TextStyle ui(TextStyle style) =>
      GoogleFonts.spaceGrotesk(textStyle: style);

  /// Família de dados (JetBrains Mono) — já com numerais tabulares.
  static TextStyle mono(TextStyle style) => GoogleFonts.jetBrainsMono(
        textStyle: style.copyWith(fontFeatures: _tabular),
      );

  // ---- Escala de tipo ----

  /// Valor herói (detalhe de PID) — 700/64 mono.
  static TextStyle get displayXl => mono(const TextStyle(
        fontSize: 64,
        fontWeight: FontWeight.w700,
        height: 1,
        letterSpacing: -0.5,
        color: AppColors.textPrimary,
      ));

  /// Valor grande ao vivo — 700/44 mono.
  static TextStyle get displayL => mono(const TextStyle(
        fontSize: 44,
        fontWeight: FontWeight.w700,
        height: 1,
        letterSpacing: -0.5,
        color: AppColors.textPrimary,
      ));

  /// Título de tela / estado — 700/26 ui.
  static TextStyle get heading => ui(const TextStyle(
        fontSize: 26,
        fontWeight: FontWeight.w700,
        height: 1.05,
        letterSpacing: -0.5,
        color: AppColors.textPrimary,
      ));

  /// Título de painel — 700/22 ui.
  static TextStyle get title => ui(const TextStyle(
        fontSize: 22,
        fontWeight: FontWeight.w700,
        height: 1.1,
        color: AppColors.textPrimary,
      ));

  /// Corpo proeminente / botão — 600/16 ui.
  static TextStyle get bodyL => ui(const TextStyle(
        fontSize: 16,
        fontWeight: FontWeight.w600,
        height: 1.4,
        color: AppColors.textPrimary,
      ));

  /// Parágrafo de instrução — 400/15 ui.
  static TextStyle get body => ui(const TextStyle(
        fontSize: 15,
        fontWeight: FontWeight.w400,
        height: 1.55,
        color: AppColors.textSecondary,
      ));

  /// Label de formulário / lista — 600/13 ui.
  static TextStyle get label => ui(const TextStyle(
        fontSize: 13,
        fontWeight: FontWeight.w600,
        height: 1.3,
        color: AppColors.textSecondary,
      ));

  /// Overline de seção — 600/11 ui, tracking +1.5px (use em UPPERCASE).
  static TextStyle get overline => ui(const TextStyle(
        fontSize: 11,
        fontWeight: FontWeight.w600,
        height: 1.3,
        letterSpacing: 1.5,
        color: AppColors.textLabel,
      ));
}
