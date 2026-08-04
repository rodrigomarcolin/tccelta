import 'package:flutter/material.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';

/// Estilo semântico do [AppButton]. A variante carrega **significado**, não só
/// ênfase.
enum AppButtonVariant {
  /// Ciano sólido — a única ação de "sinal verde" (ao vivo / ok).
  primary,

  /// Âmbar sólido — ação de estado do sistema (ex.: "Abrir ajustes").
  warning,

  /// Contorno branco/10 — ação de baixa ênfase.
  secondary,

  /// Ciano/8 + borda ciano — ação aditiva (ex.: "Adicionar ao painel").
  tonal,

  /// Texto inline (ciano ou neutro), sem fundo.
  link,
}

/// Botão de ação do cockpit.
///
/// Espelha o componente `Button` do design system. Variantes preenchidas usam
/// um token de texto escuro (on-color) para permanecerem legíveis sobre o
/// acento vivo. [fullWidth] é `true` por padrão (mobile-first).
class AppButton extends StatelessWidget {
  /// Cria um botão de ação. A [variant] carrega o significado semântico.
  const AppButton({
    required this.child,
    this.variant = AppButtonVariant.primary,
    this.fullWidth = true,
    this.icon,
    this.onPressed,
    super.key,
  });

  /// Conteúdo (normalmente um [Text]).
  final Widget child;

  /// Estilo semântico. @default [AppButtonVariant.primary]
  final AppButtonVariant variant;

  /// Estica para a largura do container. @default true
  final bool fullWidth;

  /// Ícone líder opcional (outline). Tinge na cor do texto da variante.
  final Widget? icon;

  /// `null` desabilita o botão.
  final VoidCallback? onPressed;

  bool get _filled =>
      variant == AppButtonVariant.primary ||
      variant == AppButtonVariant.warning;

  double get _height => switch (variant) {
    AppButtonVariant.primary || AppButtonVariant.warning => 54,
    AppButtonVariant.secondary || AppButtonVariant.tonal => 50,
    AppButtonVariant.link => 44, // hit target mínimo
  };

  Color get _fg => switch (variant) {
    AppButtonVariant.primary => AppColors.accentOn,
    AppButtonVariant.warning => AppColors.warningOn,
    AppButtonVariant.secondary => AppColors.textPrimary,
    AppButtonVariant.tonal || AppButtonVariant.link => AppColors.cyan500,
  };

  @override
  Widget build(BuildContext context) {
    final disabled = onPressed == null;

    final content = DefaultTextStyle.merge(
      style: AppTypography.bodyL.copyWith(color: _fg),
      child: IconTheme.merge(
        data: IconThemeData(color: _fg, size: 20),
        child: Row(
          mainAxisSize: fullWidth ? MainAxisSize.max : MainAxisSize.min,
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            if (icon != null) ...[
              icon!,
              const SizedBox(width: AppSpacing.s2),
            ],
            Flexible(child: child),
          ],
        ),
      ),
    );

    final button = _buildSurface(content);

    return Opacity(
      opacity: disabled ? 0.45 : 1,
      child: SizedBox(
        width: fullWidth ? double.infinity : null,
        height: variant == AppButtonVariant.link ? null : _height,
        child: button,
      ),
    );
  }

  Widget _buildSurface(Widget content) {
    switch (variant) {
      case AppButtonVariant.primary:
      case AppButtonVariant.warning:
        return _Pressable(
          onPressed: onPressed,
          borderRadius: AppRadii.brBtn,
          color: _filled
              ? (variant == AppButtonVariant.primary
                    ? AppColors.cyan500
                    : AppColors.amber500)
              : null,
          child: content,
        );
      case AppButtonVariant.secondary:
        return _Pressable(
          onPressed: onPressed,
          borderRadius: AppRadii.brBtn,
          border: Border.all(color: AppColors.borderStrong),
          child: content,
        );
      case AppButtonVariant.tonal:
        return _Pressable(
          onPressed: onPressed,
          borderRadius: AppRadii.brBtn,
          color: AppColors.cyan08,
          border: Border.all(color: AppColors.cyan28),
          child: content,
        );
      case AppButtonVariant.link:
        return _Pressable(
          onPressed: onPressed,
          borderRadius: AppRadii.brBtn,
          padding: const EdgeInsets.symmetric(
            vertical: AppSpacing.s4,
            horizontal: AppSpacing.s2,
          ),
          child: content,
        );
    }
  }
}

/// Superfície pressionável que dá feedback de toque: faz um *fade* de
/// opacidade e — quando há fundo/borda — escurece levemente, sem trocar a cor
/// de significado da variante.
class _Pressable extends StatefulWidget {
  const _Pressable({
    required this.child,
    required this.borderRadius,
    this.color,
    this.border,
    this.padding = const EdgeInsets.symmetric(horizontal: AppSpacing.s5),
    this.onPressed,
  });

  final Widget child;
  final BorderRadius borderRadius;
  final Color? color;
  final Border? border;
  final EdgeInsetsGeometry padding;
  final VoidCallback? onPressed;

  /// Opacidade-alvo enquanto pressionado. Padrão de "touchable": dá uma
  /// dica imediata de toque sem ser brusco.
  static const double _pressedOpacity = 0.6;

  /// Tempo mínimo que o estado pressionado fica visível. Sem isso, um toque
  /// rápido (down→up no mesmo frame) faz o fade mal começar antes de reverter,
  /// ficando imperceptível — sobretudo no link, que não tem o overlay escuro
  /// de reforço.
  static const Duration _minVisiblePress = Duration(milliseconds: 50);

  @override
  State<_Pressable> createState() => _PressableState();
}

class _PressableState extends State<_Pressable> {
  bool _down = false;
  DateTime? _downAt;

  /// Superfícies vazias (ex.: variante link) não recebem o overlay escuro —
  /// só o fade — para não desenhar um retângulo arredondado sobre texto solto.
  bool get _hasSurface => widget.color != null || widget.border != null;

  void _pressDown() {
    _downAt = DateTime.now();
    setState(() => _down = true);
  }

  /// Solta o estado pressionado, mas só depois de [_Pressable._minVisiblePress]
  /// desde o toque — garantindo que o fade complete ao menos uma vez.
  void _pressUp() {
    final held = DateTime.now().difference(_downAt ?? DateTime.now());
    final remaining = _Pressable._minVisiblePress - held;
    if (remaining <= Duration.zero) {
      setState(() => _down = false);
      return;
    }
    Future<void>.delayed(remaining, () {
      if (mounted) setState(() => _down = false);
    });
  }

  @override
  Widget build(BuildContext context) {
    final disabled = widget.onPressed == null;
    final duration = context.motion(const Duration(milliseconds: 100));
    return GestureDetector(
      onTap: disabled ? null : widget.onPressed,
      onTapDown: disabled ? null : (_) => _pressDown(),
      onTapUp: disabled ? null : (_) => _pressUp(),
      onTapCancel: disabled ? null : _pressUp,
      behavior: HitTestBehavior.opaque,
      child: AnimatedOpacity(
        duration: duration,
        opacity: _down ? _Pressable._pressedOpacity : 1,
        child: AnimatedContainer(
          duration: duration,
          alignment: Alignment.center,
          padding: widget.padding,
          decoration: BoxDecoration(
            color: widget.color,
            border: widget.border,
            borderRadius: widget.borderRadius,
          ),
          foregroundDecoration: _down && _hasSurface
              ? BoxDecoration(
                  color: const Color(0x14000000),
                  borderRadius: widget.borderRadius,
                )
              : null,
          child: widget.child,
        ),
      ),
    );
  }
}
