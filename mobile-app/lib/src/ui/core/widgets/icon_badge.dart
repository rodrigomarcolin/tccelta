import 'dart:async';

import 'package:flutter/widgets.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/ui/core/icons/app_icon.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/status_badge.dart';

/// Formato do [IconBadge].
enum IconBadgeShape {
  /// Quadrado de cantos arredondados (estados de permissão/aviso/erro).
  roundedSquare,

  /// Círculo (estado de sucesso "Conectado").
  circle,
}

/// Badge-herói grande de um estado de conexão: um [AppIcon] tonalizado dentro
/// de uma superfície wash com borda do tom.
///
/// É o "ícone do estado" no topo das telas de fluxo (permitir BT, BT desligado,
/// conectado, conexão perdida). Com [pulse], ganha um halo que "respira" —
/// sinalizando sucesso ao vivo — respeitando "reduzir movimento" do SO.
/// Reaproveita o mapa de cores de [StatusToneX].
class IconBadge extends StatefulWidget {
  /// Cria um badge para [icon] no [tone] dado.
  const IconBadge(
    this.icon, {
    this.tone = StatusTone.ok,
    this.shape = IconBadgeShape.roundedSquare,
    this.size = 104,
    this.pulse = false,
    super.key,
  });

  /// Ícone central do estado.
  final AppIconData icon;

  /// Tom semântico (define wash, borda e cor do ícone).
  /// @default [StatusTone.ok]
  final StatusTone tone;

  /// Formato da superfície. @default [IconBadgeShape.roundedSquare]
  final IconBadgeShape shape;

  /// Lado/diâmetro da superfície (px). @default 104
  final double size;

  /// Halo "respirando" atrás do badge (sucesso ao vivo). @default false
  final bool pulse;

  @override
  State<IconBadge> createState() => _IconBadgeState();
}

class _IconBadgeState extends State<IconBadge>
    with SingleTickerProviderStateMixin {
  late final AnimationController _controller = AnimationController(
    vsync: this,
    duration: AppMotion.durPulse,
  );

  late final Animation<double> _halo = _controller
      .drive(CurveTween(curve: Curves.easeInOut))
      .drive(Tween<double>(begin: 1, end: 0.3));

  bool _reduceMotion = false;

  bool get _shouldPulse => widget.pulse && !_reduceMotion;

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    _reduceMotion = context.reduceMotion;
    _syncAnimation();
  }

  @override
  void didUpdateWidget(IconBadge oldWidget) {
    super.didUpdateWidget(oldWidget);
    _syncAnimation();
  }

  void _syncAnimation() {
    if (_shouldPulse) {
      if (!_controller.isAnimating) {
        unawaited(_controller.repeat(reverse: true));
      }
    } else {
      _controller
        ..stop()
        ..value = 1;
    }
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final isCircle = widget.shape == IconBadgeShape.circle;
    final badge = Container(
      width: widget.size,
      height: widget.size,
      decoration: BoxDecoration(
        color: widget.tone.wash,
        shape: isCircle ? BoxShape.circle : BoxShape.rectangle,
        borderRadius: isCircle ? null : AppRadii.brXl,
        border: Border.all(color: widget.tone.border),
      ),
      alignment: Alignment.center,
      child: AppIcon(
        widget.icon,
        size: widget.size * 0.44,
        color: widget.tone.color,
      ),
    );

    if (!widget.pulse) return badge;

    // Halo: um círculo maior atrás do badge, "respirando" em opacidade.
    final haloSize = widget.size * 1.3;
    return SizedBox(
      width: haloSize,
      height: haloSize,
      child: Stack(
        alignment: Alignment.center,
        children: [
          FadeTransition(
            opacity: _halo,
            child: Container(
              width: haloSize,
              height: haloSize,
              decoration: BoxDecoration(
                color: widget.tone.wash,
                shape: BoxShape.circle,
              ),
            ),
          ),
          badge,
        ],
      ),
    );
  }
}
