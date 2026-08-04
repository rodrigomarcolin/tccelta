import 'package:flutter/painting.dart';

/// OBD2 Cockpit — tokens de cor.
///
/// Dark-first. Os neutros são frios (puxados para o azul). A cor é reservada
/// para significado: ciano = ao vivo / ok / ação primária, âmbar = aviso,
/// vermelho = alerta / limite, verde = "zona normal/fria" apenas.
/// Quanto maior o número do neutro, mais escuro.
///
/// Espelha `tokens/colors.css` do design system.
abstract final class AppColors {
  AppColors._();

  // ---- Neutros (frios, puxados para o azul) ----
  /// Texto primário.
  static const Color neutral50 = Color(0xFFE8EDF2);

  /// Texto secundário forte.
  static const Color neutral100 = Color(0xFFCFD6DE);

  /// Texto secundário.
  static const Color neutral200 = Color(0xFF8B97A6);

  /// Labels mono.
  static const Color neutral300 = Color(0xFF7B8794);

  /// Texto terciário / placeholder.
  static const Color neutral400 = Color(0xFF5A6675);

  /// Labels de seção.
  static const Color neutral500 = Color(0xFF44505E);

  /// Ícone inativo.
  static const Color neutral600 = Color(0xFF3A4452);

  /// Bordas / anéis inativos.
  static const Color neutral700 = Color(0xFF2A3441);

  /// Trilha de gauge / barra.
  static const Color neutral800 = Color(0xFF161E29);

  /// Superfície de card.
  static const Color neutral850 = Color(0xFF141B24);

  /// Superfície de lista.
  static const Color neutral880 = Color(0xFF10161E);

  /// Fundo de tela.
  static const Color neutral900 = Color(0xFF0A0E14);

  /// Moldura do device / preto verdadeiro.
  static const Color neutral950 = Color(0xFF05080D);

  // ---- Ciano — ao vivo / ok / ação primária ----
  /// Ciano claro (hover / realce).
  static const Color cyan400 = Color(0xFF5FE3F5);

  /// O acento da marca.
  static const Color cyan500 = Color(0xFF22D3EE);

  /// Texto sobre ciano sólido (botões/badges).
  static const Color cyan950 = Color(0xFF04222A);

  // ---- Âmbar — aviso / atenção ----
  /// Âmbar de aviso.
  static const Color amber500 = Color(0xFFFFB020);

  /// Texto sobre âmbar sólido.
  static const Color amber950 = Color(0xFF241A06);

  // ---- Vermelho — alerta / erro / limite ----
  /// Vermelho de alerta/erro.
  static const Color red500 = Color(0xFFF4385A);

  /// Texto sobre superfícies translúcidas vermelhas.
  static const Color red200 = Color(0xFFE09AA6);

  // ---- Verde — "zona normal/fria" apenas (nunca uma ação) ----
  /// Verde da zona "normal/fria" de gauge.
  static const Color green500 = Color(0xFF1F8A5B);

  // ---- Aliases semânticos ----
  /// Fundo de tela.
  static const Color bgScreen = neutral900;

  /// Moldura do device.
  static const Color bgFrame = neutral950;

  /// Superfície de tiles/cards.
  static const Color surfaceCard = neutral850;

  /// Superfície de linhas de lista.
  static const Color surfaceList = neutral880;

  /// Superfície rebaixada.
  static const Color surfaceSunken = neutral900;

  /// Trilha de gauge/barra.
  static const Color track = neutral800;

  /// Texto primário.
  static const Color textPrimary = neutral50;

  /// Texto secundário.
  static const Color textSecondary = neutral200;

  /// Texto terciário / placeholder.
  static const Color textTertiary = neutral400;

  /// Label de seção.
  static const Color textLabel = neutral500;

  /// Borda interna de card — `rgba(255,255,255,.05)`.
  static const Color borderHairline = Color(0x0DFFFFFF);

  /// Anel da moldura do device — `rgba(255,255,255,.04)`.
  static const Color borderFrame = Color(0x0AFFFFFF);

  /// Botões com contorno — `rgba(255,255,255,.10)`.
  static const Color borderStrong = Color(0x1AFFFFFF);

  /// Cor de acento (ação primária / ao vivo).
  static const Color accent = cyan500;

  /// Texto/ícone sobre acento sólido.
  static const Color accentOn = cyan950;

  /// Cor de aviso.
  static const Color warning = amber500;

  /// Texto/ícone sobre aviso sólido.
  static const Color warningOn = amber950;

  /// Cor de alerta/erro.
  static const Color danger = red500;

  // ---- Washes translúcidos de acento (cor a NN% de alpha) ----
  /// Ciano a 6%.
  static const Color cyan06 = Color(0x0F22D3EE);

  /// Ciano a 8%.
  static const Color cyan08 = Color(0x1422D3EE);

  /// Ciano a 10%.
  static const Color cyan10 = Color(0x1A22D3EE);

  /// Ciano a 14%.
  static const Color cyan14 = Color(0x2422D3EE);

  /// Ciano a 28%.
  static const Color cyan28 = Color(0x4722D3EE);

  /// Âmbar a 8%.
  static const Color amber08 = Color(0x14FFB020);

  /// Âmbar a 20%.
  static const Color amber20 = Color(0x33FFB020);

  /// Vermelho a 8%.
  static const Color red08 = Color(0x14F4385A);

  /// Vermelho a 20%.
  static const Color red20 = Color(0x33F4385A);

  /// Vermelho a 32%.
  static const Color red32 = Color(0x52F4385A);

  /// Fundo translúcido da tab bar — `rgba(8,12,18,.94)`.
  static const Color tabBarSurface = Color(0xF0080C12); // ~94%
}
