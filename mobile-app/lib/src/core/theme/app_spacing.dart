import 'package:flutter/widgets.dart';

/// OBD2 Cockpit — tokens de espaçamento.
///
/// Um conjunto pequeno e intencional — ritmo apertado e denso, de cluster de
/// instrumentos. Espelha `tokens/spacing.css`.
abstract final class AppSpacing {
  AppSpacing._();

  /// Gap fino.
  static const double s2 = 8;

  /// Gap de grid.
  static const double s3 = 10;

  /// Gap de lista.
  static const double s4 = 12;

  /// Padding de card.
  static const double s5 = 14;

  /// Margem de tela.
  static const double s7 = 20;

  /// Padding de seção.
  static const double s9 = 28;
}

/// OBD2 Cockpit — tokens de raio.
///
/// Os raios sobem das pílulas internas até o canto da tela do device.
/// Espelha `tokens/spacing.css`.
abstract final class AppRadii {
  AppRadii._();

  /// Badge / pílula interna.
  static const double sm = 8;

  /// Item de lista.
  static const double md = 12;

  /// Botão.
  static const double btn = 15;

  /// Card.
  static const double lg = 16;

  /// Card grande.
  static const double xl = 24;

  /// Canto da tela do device.
  static const double screen = 42;

  /// Pílulas de status / dots.
  static const double pill = 999;

  // Helpers prontos de BorderRadius.
  /// [BorderRadius] do raio [sm] (badge / pílula interna).
  static const BorderRadius brSm = BorderRadius.all(Radius.circular(sm));

  /// [BorderRadius] do raio [md] (item de lista).
  static const BorderRadius brMd = BorderRadius.all(Radius.circular(md));

  /// [BorderRadius] do raio [btn] (botão).
  static const BorderRadius brBtn = BorderRadius.all(Radius.circular(btn));

  /// [BorderRadius] do raio [lg] (card).
  static const BorderRadius brLg = BorderRadius.all(Radius.circular(lg));

  /// [BorderRadius] do raio [xl] (card grande).
  static const BorderRadius brXl = BorderRadius.all(Radius.circular(xl));

  /// [BorderRadius] do raio [pill] (pílulas / dots).
  static const BorderRadius brPill = BorderRadius.all(Radius.circular(pill));
}
