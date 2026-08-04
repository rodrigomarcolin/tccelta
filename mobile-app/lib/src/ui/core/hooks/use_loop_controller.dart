import 'dart:async';

import 'package:flutter/widgets.dart';
import 'package:flutter_hooks/flutter_hooks.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';

/// Hook: um [AnimationController] em loop que respeita "reduzir movimento".
///
/// Encapsula o padrão repetido dos átomos animados (radar, spinner, pontinhos):
/// cria o controller (com `vsync` gerido pelo próprio hook), e faz `repeat()`
/// ou `stop()` conforme a preferência de acessibilidade do SO
/// ([MotionX.reduceMotion]) e o parâmetro [active].
///
/// Passe [active] `false` para congelar o loop sob demanda (ex.: radar parado
/// quando não está escaneando) — o valor corrente fica preservado. Como
/// `reduceMotion` e `active` entram nas dependências do [useEffect], o loop
/// reage automaticamente a mudanças em runtime — sem `didChangeDependencies`.
AnimationController useLoopController(Duration duration, {bool active = true}) {
  final controller = useAnimationController(duration: duration);
  final reduceMotion = useContext().reduceMotion;
  useEffect(
    () {
      if (reduceMotion || !active) {
        controller.stop();
      } else {
        unawaited(controller.repeat());
      }
      return null;
    },
    [controller, reduceMotion, active],
  );
  return controller;
}
