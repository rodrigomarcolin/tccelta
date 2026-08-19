import 'package:flutter/material.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';

/// Casca comum das telas do fluxo de conexão.
///
/// Um [Scaffold] dark com um gradiente radial sutil no topo, tingido pelo
/// [tone] do estado (ciano = neutro/sucesso, âmbar = aviso, vermelho = erro) —
/// espelhando os `radial-gradient(... at 50% 0%)` do design. O [child] já vem
/// dentro de uma [SafeArea].
class ConnectionBackground extends StatelessWidget {
  /// Cria a casca com o [tone] dado envolvendo [child].
  const ConnectionBackground({
    required this.child,
    this.tone = StatusTone.ok,
    super.key,
  });

  /// Conteúdo da tela (dentro da `SafeArea`).
  final Widget child;

  /// Tom do estado — define a cor do topo do gradiente.
  /// @default [StatusTone.ok]
  final StatusTone tone;

  // Topo do gradiente radial por tom. São cores de *fundo de tela* específicas
  // do frame (não tokens reusáveis): espelham os tops dos radial-gradients do
  // design. A base é sempre [AppColors.bgScreen].
  Color get _gradientTop => switch (tone) {
    StatusTone.live || StatusTone.ok => const Color(0xFF121A24),
    StatusTone.warning => const Color(0xFF1C1810),
    StatusTone.alert => const Color(0xFF1A1014),
  };

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: AppColors.bgScreen,
      body: DecoratedBox(
        decoration: BoxDecoration(
          gradient: RadialGradient(
            center: Alignment.topCenter,
            radius: 1.2,
            colors: [_gradientTop, AppColors.bgScreen],
            stops: const [0, 0.6],
          ),
        ),
        child: SafeArea(child: child),
      ),
    );
  }
}
