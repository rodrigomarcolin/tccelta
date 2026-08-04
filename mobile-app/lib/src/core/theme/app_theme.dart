import 'package:flutter/material.dart';
import 'package:tccelta_mobile/src/core/theme/app_colors.dart';
import 'package:tccelta_mobile/src/core/theme/app_spacing.dart';
import 'package:tccelta_mobile/src/core/theme/app_typography.dart';

/// OBD2 Cockpit — montagem do [ThemeData].
///
/// Reúne os tokens ([AppColors], [AppTypography], [AppSpacing], [AppRadii]) num
/// tema escuro Material 3, para que widgets nativos herdem a marca por padrão.
/// Os átomos do design system continuam lendo os tokens diretamente — este tema
/// é a base coerente sob eles.
abstract final class AppTheme {
  AppTheme._();

  /// Tema escuro do OBD2 Cockpit (o único tema — o app é dark-first).
  static ThemeData get dark {
    const colorScheme = ColorScheme.dark(
      primary: AppColors.cyan500,
      onPrimary: AppColors.cyan950,
      secondary: AppColors.cyan400,
      onSecondary: AppColors.cyan950,
      surface: AppColors.surfaceCard,
      onSurface: AppColors.textPrimary,
      error: AppColors.red500,
      onError: AppColors.cyan950,
      outline: AppColors.neutral700,
    );

    final textTheme = TextTheme(
      displayLarge: AppTypography.displayXl,
      displayMedium: AppTypography.displayL,
      headlineMedium: AppTypography.heading,
      titleLarge: AppTypography.title,
      bodyLarge: AppTypography.bodyL,
      bodyMedium: AppTypography.body,
      labelLarge: AppTypography.label,
      labelSmall: AppTypography.overline,
    );

    return ThemeData(
      useMaterial3: true,
      brightness: Brightness.dark,
      scaffoldBackgroundColor: AppColors.bgScreen,
      colorScheme: colorScheme,
      textTheme: textTheme,
      canvasColor: AppColors.bgScreen,
      dividerColor: AppColors.borderHairline,
      splashColor: AppColors.cyan10,
      highlightColor: AppColors.cyan08,
      iconTheme: const IconThemeData(color: AppColors.textPrimary, size: 22),
      cardTheme: const CardThemeData(
        color: AppColors.surfaceCard,
        elevation: 0,
        margin: EdgeInsets.zero,
        shape: RoundedRectangleBorder(
          borderRadius: AppRadii.brLg,
          side: BorderSide(color: AppColors.borderHairline),
        ),
      ),
    );
  }
}
